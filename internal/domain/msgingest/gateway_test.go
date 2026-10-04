package msgingest_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// mockMsgStore implements msgingest.MessageStore for testing.
type mockMsgStore struct {
	batches       [][]store.BatchMsg // one entry per CreateBatch call
	rowsToReturn  int64              // rows returned when useCustomRows is true
	errToReturn   error
	useCustomRows bool
}

func newMock() *mockMsgStore { return &mockMsgStore{} }

func (m *mockMsgStore) withPartialInsert(rows int64) *mockMsgStore {
	m.useCustomRows = true
	m.rowsToReturn = rows
	return m
}

func (m *mockMsgStore) withError(err error) *mockMsgStore {
	m.errToReturn = err
	return m
}

func (m *mockMsgStore) CreateBatch(_ context.Context, msgs []store.BatchMsg) (int64, error) {
	if m.errToReturn != nil {
		return 0, m.errToReturn
	}
	cp := make([]store.BatchMsg, len(msgs))
	copy(cp, msgs)
	m.batches = append(m.batches, cp)
	if m.useCustomRows {
		return m.rowsToReturn, nil
	}
	return int64(len(msgs)), nil
}

// noopHandler is a pass-through CommandHandler for tests that don't exercise command handling.
type noopHandler struct{}

func (noopHandler) IsCommand(_ string) bool { return false }
func (noopHandler) HandleCommand(_ context.Context, _ string, _ platform.InboundMessage) bool {
	return false
}

func inbound(sessionKey, content, platformMsgID string) platform.InboundMessage {
	return platform.InboundMessage{
		Platform:          "test",
		SessionKey:        sessionKey,
		Content:           content,
		PlatformMessageID: platformMsgID,
	}
}

// TestGateway_Dedup_InMemory verifies that two dispatches with the same
// platform_msg_id in one debounce window result in exactly one row written.
func TestGateway_Dedup_InMemory(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 150*time.Millisecond, noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "hello", "pmsg-1"))
	g.Dispatch(inbound("s1", "world", "pmsg-1")) // same platform_msg_id → dropped

	select {
	case <-g.Out():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced message")
	}

	require.Len(t, st.batches, 1)
	require.Len(t, st.batches[0], 1, "duplicate dropped")
}

// TestGateway_DebounceMerge merges the debounce-window scenarios: plain
// messages and a "clear" message, with and without a preceding merge.
func TestGateway_DebounceMerge(t *testing.T) {
	cases := []struct {
		name         string
		debounce     time.Duration
		msgs         []platform.InboundMessage
		wantContent  string
		waitTimeout  time.Duration
		checkNoExtra bool
		extraTimeout time.Duration
	}{
		{
			name:     "EmitsSingleMergedMessage",
			debounce: 100 * time.Millisecond,
			msgs: []platform.InboundMessage{
				inbound("s1", "hello", "m1"),
				inbound("s1", "world", "m2"),
			},
			wantContent:  "hello\n\n---\n\nworld",
			waitTimeout:  500 * time.Millisecond,
			checkNoExtra: true,
			extraTimeout: 200 * time.Millisecond,
		},
		{
			name:        "ClearMessage_DebounceAsNormal",
			debounce:    100 * time.Millisecond,
			msgs:        []platform.InboundMessage{inbound("s1", "clear", "cmd-1")},
			wantContent: "clear",
			waitTimeout: 500 * time.Millisecond,
		},
		{
			name:     "ClearMessage_MergedWithDebounce",
			debounce: 200 * time.Millisecond,
			msgs: []platform.InboundMessage{
				inbound("s1", "hello", "m1"),
				inbound("s1", "clear", "cmd-1"),
			},
			wantContent:  "hello\n\n---\n\nclear",
			waitTimeout:  500 * time.Millisecond,
			checkNoExtra: true,
			extraTimeout: 300 * time.Millisecond,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newMock()
			g := msgingest.New(st, tc.debounce, noopHandler{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go g.Run(ctx)

			for _, m := range tc.msgs {
				g.Dispatch(m)
			}

			select {
			case msg := <-g.Out():
				require.Equal(t, tc.wantContent, msg.Content)
			case <-time.After(tc.waitTimeout):
				t.Fatal("timeout waiting for debounced message")
			}

			if tc.checkNoExtra {
				select {
				case extra := <-g.Out():
					t.Fatalf("expected only one message, got extra: %+v", extra)
				case <-time.After(tc.extraTimeout):
				}
			}
		})
	}
}

// TestGateway_Debounce_BatchWrite verifies the exact structure of the
// CreateBatch call: 2 merged rows + 1 received row, correct MergedInto.
func TestGateway_Debounce_BatchWrite(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "msg1", "m1"))
	g.Dispatch(inbound("s1", "msg2", "m2"))
	g.Dispatch(inbound("s1", "msg3", "m3"))

	var emitted msgingest.IngestedMessage
	select {
	case emitted = <-g.Out():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced message")
	}

	require.Len(t, st.batches, 1)
	batch := st.batches[0]
	require.Len(t, batch, 3)

	// Last row is the primary (received)
	primary := batch[2]
	assert.Equal(t, "received", primary.Status)
	assert.Empty(t, primary.MergedInto)
	assert.Equal(t, emitted.MsgID, primary.ID)

	// First two rows are merged
	for _, row := range batch[:2] {
		assert.Equal(t, "merged", row.Status)
		assert.Equal(t, primary.ID, row.MergedInto)
	}
}

// TestGateway_Debounce_SingleMessage verifies N=1: CreateBatch called with
// exactly one received row and no merged rows.
func TestGateway_Debounce_SingleMessage(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "only", "m1"))

	select {
	case <-g.Out():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout")
	}

	require.Len(t, st.batches, 1)
	require.Len(t, st.batches[0], 1)
	assert.Equal(t, "received", st.batches[0][0].Status)
}

// TestGateway_BatchWrite_Error_NormalPath verifies that a CreateBatch error
// during debounce suppresses the emit.
func TestGateway_BatchWrite_Error_NormalPath(t *testing.T) {
	st := newMock().withError(errors.New("db down"))
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "hello", "m1"))

	select {
	case msg := <-g.Out():
		t.Fatalf("expected no emit on CreateBatch error, got: %+v", msg)
	case <-time.After(400 * time.Millisecond):
		// expected: nothing emitted
	}
}

// TestGateway_BatchWrite_PartialInsert verifies that rowsInserted < N suppresses
// the emit (covers the case where the primary is ignored but merged rows succeed).
func TestGateway_BatchWrite_PartialInsert(t *testing.T) {
	// 3 messages dispatched → batch of 3; mock returns only 2 inserted
	st := newMock().withPartialInsert(2)
	g := msgingest.New(st, 100*time.Millisecond, noopHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "msg1", "m1"))
	g.Dispatch(inbound("s1", "msg2", "m2"))
	g.Dispatch(inbound("s1", "msg3", "m3"))

	select {
	case msg := <-g.Out():
		t.Fatalf("expected no emit on partial insert, got: %+v", msg)
	case <-time.After(400 * time.Millisecond):
		// expected
	}
}

// mockCommandHandler records whether HandleCommand was called and what to return.
type mockCommandHandler struct {
	mu        sync.Mutex
	handled   bool
	contents  []string
	called    chan struct{} // closed on first HandleCommand invocation
	closeOnce sync.Once
}

func newMockCommandHandler(handled bool) *mockCommandHandler {
	return &mockCommandHandler{
		handled: handled,
		called:  make(chan struct{}),
	}
}

func (m *mockCommandHandler) IsCommand(_ string) bool { return m.handled }

func (m *mockCommandHandler) HandleCommand(_ context.Context, content string, _ platform.InboundMessage) bool {
	m.mu.Lock()
	m.contents = append(m.contents, content)
	m.mu.Unlock()
	m.closeOnce.Do(func() { close(m.called) })
	return m.handled
}

func (m *mockCommandHandler) getContents() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.contents))
	copy(cp, m.contents)
	return cp
}

// TestGateway_Command merges the command-handling scenarios: interception
// before any DB write, pass-through for non-commands, and bypassing the
// debounce window entirely.
func TestGateway_Command(t *testing.T) {
	cases := []struct {
		name         string
		debounce     time.Duration
		handled      bool
		msg          platform.InboundMessage
		waitTimeout  time.Duration
		wantBatches  int
		wantContents []string // nil => skip
		checkFast    bool     // BypassesDebounce: assert the handler fires well under the debounce window
	}{
		{
			name:         "InterceptsBeforeDB",
			debounce:     0,
			handled:      true,
			msg:          platform.InboundMessage{Platform: "feishu", SessionKey: "feishu:c1:u1", Content: "/engine claude"},
			waitTimeout:  500 * time.Millisecond,
			wantBatches:  0,
			wantContents: []string{"/engine claude"},
		},
		{
			name:        "PassesThroughNonCommands",
			debounce:    0,
			handled:     false,
			msg:         platform.InboundMessage{Platform: "feishu", SessionKey: "feishu:c1:u1", Content: "hello"},
			waitTimeout: 500 * time.Millisecond,
			wantBatches: 1,
		},
		{
			name:        "BypassesDebounce",
			debounce:    500 * time.Millisecond,
			handled:     true,
			msg:         platform.InboundMessage{Platform: "feishu", SessionKey: "feishu:c1:u1", Content: "/engine claude"},
			waitTimeout: 200 * time.Millisecond,
			wantBatches: 0,
			checkFast:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newMock()
			handler := newMockCommandHandler(tc.handled)
			g := msgingest.New(st, tc.debounce, handler)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go g.Run(ctx)

			start := time.Now()
			g.Dispatch(tc.msg)

			if tc.handled {
				select {
				case <-handler.called:
				case <-time.After(tc.waitTimeout):
					t.Fatal("timeout waiting for command handler to be called")
				}
			} else {
				select {
				case <-g.Out():
				case <-time.After(tc.waitTimeout):
					t.Fatal("timeout waiting for message to be emitted")
				}
			}

			if tc.checkFast {
				assert.Less(t, time.Since(start), 200*time.Millisecond)
			}
			assert.Len(t, st.batches, tc.wantBatches)
			if tc.wantContents != nil {
				assert.Equal(t, tc.wantContents, handler.getContents())
			}
		})
	}
}

// TestGateway_BotMention merges the bot-mention-stripping scenarios for a
// single message and for messages merged by the debounce window.
func TestGateway_BotMention(t *testing.T) {
	cases := []struct {
		name          string
		debounce      time.Duration
		botNames      map[string]string
		msgs          []platform.InboundMessage
		waitTimeout   time.Duration
		wantContent   string
		wantBatchRows []string // expected Content per batch row, in order
	}{
		{
			name:          "StrippedInEmitAndDB",
			debounce:      100 * time.Millisecond,
			botNames:      map[string]string{"test": "OpenBee"},
			msgs:          []platform.InboundMessage{inbound("s1", "@OpenBee hello world", "m1")},
			waitTimeout:   500 * time.Millisecond,
			wantContent:   "hello world",
			wantBatchRows: []string{"hello world"},
		},
		{
			name:     "MergedMessagesStripped",
			debounce: 150 * time.Millisecond,
			botNames: map[string]string{"test": "Bot"},
			msgs: []platform.InboundMessage{
				inbound("s1", "@Bot hello", "m1"),
				inbound("s1", "world @Bot", "m2"),
			},
			waitTimeout:   500 * time.Millisecond,
			wantContent:   "hello\n\n---\n\nworld",
			wantBatchRows: []string{"hello", "world"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newMock()
			g := msgingest.New(st, tc.debounce, noopHandler{}, msgingest.WithPlatformBotNames(tc.botNames))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go g.Run(ctx)

			for _, m := range tc.msgs {
				g.Dispatch(m)
			}

			var emitted msgingest.IngestedMessage
			select {
			case emitted = <-g.Out():
			case <-time.After(tc.waitTimeout):
				t.Fatal("timeout waiting for debounced message")
			}

			assert.Equal(t, tc.wantContent, emitted.Content)
			require.Len(t, st.batches, 1)                        // guard: avoid index panic on st.batches[0]
			require.Len(t, st.batches[0], len(tc.wantBatchRows)) // guard: avoid index panic below
			for i, want := range tc.wantBatchRows {
				assert.Equal(t, want, st.batches[0][i].Content)
			}
		})
	}
}
