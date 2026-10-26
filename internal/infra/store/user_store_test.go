package store

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUserStore(t *testing.T) *UserStore {
	t.Helper()
	return NewUserStore(newTestDB(t))
}

func TestUserStore_CreateFirstAndAuthenticate(t *testing.T) {
	us := setupUserStore(t)
	u, err := us.CreateFirst("alice", "s3cret", "Alice")
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)

	got, err := us.Authenticate("alice", "s3cret")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID, "authenticated wrong user")
	require.Equal(t, "Alice", got.DisplayName)

	_, err = us.Authenticate("alice", "wrong")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	_, err = us.Authenticate("bob", "s3cret")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestUserStore_Count(t *testing.T) {
	us := setupUserStore(t)
	n, err := us.Count()
	require.NoError(t, err)
	require.Equal(t, 0, n)
	_, err = us.CreateFirst("bob", "pw", "Bob")
	require.NoError(t, err)
	n, err = us.Count()
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestUserStore_SetPassword(t *testing.T) {
	us := setupUserStore(t)
	u, err := us.CreateFirst("dave", "pw", "Dave")
	require.NoError(t, err)

	require.NoError(t, us.SetPassword(u.ID, "newpw"))
	_, err = us.Authenticate("dave", "pw")
	require.Error(t, err, "old password must stop working")
	_, err = us.Authenticate("dave", "newpw")
	require.NoError(t, err, "expected new password to work")
}

func TestUserStore_SetPasswordBumpsPasswordChangedAt(t *testing.T) {
	us := setupUserStore(t)
	u, err := us.CreateFirst("erin", "pw", "Erin")
	require.NoError(t, err)

	before, err := us.PasswordChangedAt(u.ID)
	require.NoError(t, err)

	require.NoError(t, us.SetPassword(u.ID, "newpw"))
	after, err := us.PasswordChangedAt(u.ID)
	require.NoError(t, err, "PasswordChangedAt after")
	require.True(t, after >= before, "password_changed_at must advance on SetPassword: before=%d after=%d", before, after)
	require.True(t, after%1000 == 0, "password_changed_at must be floored to the second, got %d", after)
}

func TestUserStore_PasswordChangedAtUnknownUser(t *testing.T) {
	us := setupUserStore(t)
	_, err := us.PasswordChangedAt("missing")
	require.Error(t, err)
}

func TestUserStore_AdminID(t *testing.T) {
	us := setupUserStore(t)

	_, ok, err := us.AdminID()
	require.NoError(t, err)
	require.False(t, ok)

	u, err := us.CreateFirst("root", "s3cret", "")
	require.NoError(t, err)

	id, ok, err := us.AdminID()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, u.ID, id)
}

func TestUserStore_CreateFirstAllowsOnlyOneUser(t *testing.T) {
	us := setupUserStore(t)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = us.CreateFirst(fmt.Sprintf("admin-%d", i), "s3cret", "")
		}()
	}
	wg.Wait()

	created := 0
	for _, err := range errs {
		if err == nil {
			created++
			continue
		}
		assert.ErrorIs(t, err, ErrAlreadyInitialized)
	}
	assert.Equal(t, 1, created)
	n, err := us.Count()
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
