package msgingest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/msgingest"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
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
	g := msgingest.New(st, 150*time.Millisecond)
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

	if len(st.batches) != 1 {
		t.Fatalf("expected 1 CreateBatch call, got %d", len(st.batches))
	}
	if len(st.batches[0]) != 1 {
		t.Fatalf("expected 1 row in batch (duplicate dropped), got %d", len(st.batches[0]))
	}
}

// TestGateway_Debounce_EmitsSingleMergedMessage verifies that two messages in
// one debounce window are merged into one IngestedMessage with combined content.
func TestGateway_Debounce_EmitsSingleMergedMessage(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "hello", "m1"))
	g.Dispatch(inbound("s1", "world", "m2"))

	select {
	case msg := <-g.Out():
		if msg.Content != "hello\n\n---\n\nworld" {
			t.Fatalf("expected merged content, got %q", msg.Content)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced message")
	}

	// Only one message
	select {
	case extra := <-g.Out():
		t.Fatalf("expected only one message, got extra: %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestGateway_Debounce_BatchWrite verifies the exact structure of the
// CreateBatch call: 2 merged rows + 1 received row, correct MergedInto.
func TestGateway_Debounce_BatchWrite(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond)
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

	if len(st.batches) != 1 {
		t.Fatalf("expected 1 CreateBatch call, got %d", len(st.batches))
	}
	batch := st.batches[0]
	if len(batch) != 3 {
		t.Fatalf("expected 3 rows in batch, got %d", len(batch))
	}

	// Last row is the primary (received)
	primary := batch[2]
	if primary.Status != "received" {
		t.Errorf("primary row: want status=received, got %q", primary.Status)
	}
	if primary.MergedInto != "" {
		t.Errorf("primary row: want merged_into empty, got %q", primary.MergedInto)
	}
	if primary.ID != emitted.MsgID {
		t.Errorf("primary ID %q != emitted MsgID %q", primary.ID, emitted.MsgID)
	}

	// First two rows are merged
	for i, row := range batch[:2] {
		if row.Status != "merged" {
			t.Errorf("row[%d]: want status=merged, got %q", i, row.Status)
		}
		if row.MergedInto != primary.ID {
			t.Errorf("row[%d]: want merged_into=%q, got %q", i, primary.ID, row.MergedInto)
		}
	}
}

// TestGateway_Debounce_SingleMessage verifies N=1: CreateBatch called with
// exactly one received row and no merged rows.
func TestGateway_Debounce_SingleMessage(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "only", "m1"))

	select {
	case <-g.Out():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout")
	}

	if len(st.batches) != 1 {
		t.Fatalf("expected 1 CreateBatch call, got %d", len(st.batches))
	}
	if len(st.batches[0]) != 1 {
		t.Fatalf("expected 1 row, got %d", len(st.batches[0]))
	}
	if st.batches[0][0].Status != "received" {
		t.Errorf("want status=received, got %q", st.batches[0][0].Status)
	}
}

// TestGateway_BatchWrite_Error_NormalPath verifies that a CreateBatch error
// during debounce suppresses the emit.
func TestGateway_BatchWrite_Error_NormalPath(t *testing.T) {
	st := newMock().withError(errors.New("db down"))
	g := msgingest.New(st, 100*time.Millisecond)
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
	g := msgingest.New(st, 100*time.Millisecond)
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

// TestGateway_ClearMessage_DebounceAsNormal verifies that a "clear" message is
// debounced normally (no special command handling).
func TestGateway_ClearMessage_DebounceAsNormal(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "clear", "cmd-1"))

	select {
	case msg := <-g.Out():
		if msg.Content != "clear" {
			t.Fatalf("expected content 'clear', got %q", msg.Content)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced message")
	}
}

// TestGateway_ClearMessage_MergedWithDebounce verifies that "clear" sent after
// a normal message within the debounce window is merged into one message.
func TestGateway_ClearMessage_MergedWithDebounce(t *testing.T) {
	st := newMock()
	g := msgingest.New(st, 200*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	g.Dispatch(inbound("s1", "hello", "m1"))
	g.Dispatch(inbound("s1", "clear", "cmd-1"))

	select {
	case msg := <-g.Out():
		if msg.Content != "hello\n\n---\n\nclear" {
			t.Fatalf("expected merged content, got %q", msg.Content)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced message")
	}

	// No extra messages
	select {
	case extra := <-g.Out():
		t.Fatalf("unexpected extra message: %+v", extra)
	case <-time.After(300 * time.Millisecond):
	}
}
