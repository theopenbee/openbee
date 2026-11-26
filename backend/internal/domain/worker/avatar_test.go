package worker

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type fakeAvatars struct {
	refs     map[string]bool
	lastUsed map[string]bool
	pick     string
}

func (f *fakeAvatars) HasAvatar(ref string) bool { return f.refs[ref] }

func (f *fakeAvatars) RandomAvatar(used map[string]bool) string {
	f.lastUsed = used
	return f.pick
}

func newAvatarManager(t *testing.T) (*Manager, *fakeAvatars) {
	t.Helper()
	m := newTestManager(t, map[string]ai.EngineAdapter{}, "")
	src := &fakeAvatars{refs: map[string]bool{"mfers/a": true, "mfers/b": true}, pick: "mfers/b"}
	m.SetAvatarSource(src)
	return m, src
}

func TestCreateWorker_PicksRandomAvatar(t *testing.T) {
	m, src := newAvatarManager(t)
	_, err := m.CreateWorker(CreateWorkerParams{Name: "first", Avatar: "mfers/a"})
	require.NoError(t, err)

	w, err := m.CreateWorker(CreateWorkerParams{Name: "second"})
	require.NoError(t, err)
	assert.Equal(t, "mfers/b", w.Avatar)
	assert.Equal(t, map[string]bool{"mfers/a": true}, src.lastUsed)

	stored, err := m.workerStore.GetByID(w.ID)
	require.NoError(t, err)
	assert.Equal(t, "mfers/b", stored.Avatar)
}

func TestCreateWorker_NoAvatarSource(t *testing.T) {
	m := newTestManager(t, map[string]ai.EngineAdapter{}, "")
	w, err := m.CreateWorker(CreateWorkerParams{Name: "plain"})
	require.NoError(t, err)
	assert.Empty(t, w.Avatar)

	_, err = m.CreateWorker(CreateWorkerParams{Name: "picky", Avatar: "mfers/a"})
	assert.True(t, errors.Is(err, ErrValidation))
}

func TestCreateWorker_RejectsUnknownAvatar(t *testing.T) {
	m, _ := newAvatarManager(t)
	_, err := m.CreateWorker(CreateWorkerParams{Name: "x", Avatar: "mfers/zz"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation))
	assert.Contains(t, err.Error(), "mfers/zz")
}

func TestUpdateWorker_Avatar(t *testing.T) {
	m, src := newAvatarManager(t)
	w, err := m.CreateWorker(CreateWorkerParams{Name: "x", Avatar: "mfers/a"})
	require.NoError(t, err)

	b := "mfers/b"
	got, err := m.UpdateWorker(w.ID, UpdateWorkerParams{Avatar: &b})
	require.NoError(t, err)
	assert.Equal(t, "mfers/b", got.Avatar)

	bad := "mfers/zz"
	_, err = m.UpdateWorker(w.ID, UpdateWorkerParams{Avatar: &bad})
	assert.True(t, errors.Is(err, ErrValidation))

	delete(src.refs, "mfers/b")
	name := "renamed"
	got, err = m.UpdateWorker(w.ID, UpdateWorkerParams{Name: &name, Avatar: &b})
	require.NoError(t, err)
	assert.Equal(t, "mfers/b", got.Avatar)

	empty := ""
	got, err = m.UpdateWorker(w.ID, UpdateWorkerParams{Avatar: &empty})
	require.NoError(t, err)
	assert.Empty(t, got.Avatar)
	stored, err := m.workerStore.GetByID(w.ID)
	require.NoError(t, err)
	assert.Empty(t, stored.Avatar)
}

type countingAvatars struct {
	refs []string
}

func (c *countingAvatars) HasAvatar(string) bool { return true }

func (c *countingAvatars) RandomAvatar(used map[string]bool) string {
	for _, ref := range c.refs {
		if !used[ref] {
			return ref
		}
	}
	return c.refs[0]
}

func TestCreateWorker_ConcurrentCreatesGetDistinctAvatars(t *testing.T) {
	m := newTestManager(t, map[string]ai.EngineAdapter{}, "")
	refs := make([]string, 8)
	for i := range refs {
		refs[i] = fmt.Sprintf("mfers/%d", i)
	}
	m.SetAvatarSource(&countingAvatars{refs: refs})

	var wg sync.WaitGroup
	got := make([]string, len(refs))
	for i := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := m.CreateWorker(CreateWorkerParams{Name: fmt.Sprintf("w%d", i)})
			require.NoError(t, err)
			got[i] = w.Avatar
		}()
	}
	wg.Wait()
	assert.ElementsMatch(t, refs, got)
}
