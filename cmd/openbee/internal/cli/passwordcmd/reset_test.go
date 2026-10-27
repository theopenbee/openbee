package passwordcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestMain(m *testing.M) {
	if err := i18n.Load("en"); err != nil {
		panic("i18n.Load: " + err.Error())
	}
	os.Exit(m.Run())
}

func noPrompt(t *testing.T) func() (string, error) {
	return func() (string, error) {
		t.Fatal("prompt must not be called")
		return "", nil
	}
}

func newDB(t *testing.T, withAdmin bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openbee.db")
	db, err := store.InitDB(path)
	require.NoError(t, err)
	defer db.Close()
	if withAdmin {
		_, err = store.NewUserStore(db).CreateFirst("root", "old-secret", "Root")
		require.NoError(t, err)
	}
	return path
}

func authenticate(t *testing.T, path, password string) error {
	t.Helper()
	db, err := store.InitDB(path)
	require.NoError(t, err)
	defer db.Close()
	_, err = store.NewUserStore(db).Authenticate("root", password)
	return err
}

func TestResetPassword_WithFlag(t *testing.T) {
	path := newDB(t, true)
	var out bytes.Buffer

	require.NoError(t, resetPassword(&out, path, "new-secret", noPrompt(t)))

	assert.Contains(t, out.String(), `"root"`)
	assert.NoError(t, authenticate(t, path, "new-secret"))
	assert.ErrorIs(t, authenticate(t, path, "old-secret"), store.ErrInvalidCredentials)
}

func TestResetPassword_PromptsWhenFlagEmpty(t *testing.T) {
	path := newDB(t, true)
	prompted := false
	prompt := func() (string, error) {
		prompted = true
		return "typed-secret", nil
	}

	require.NoError(t, resetPassword(&bytes.Buffer{}, path, "", prompt))

	assert.True(t, prompted)
	assert.NoError(t, authenticate(t, path, "typed-secret"))
}

func TestResetPassword_PromptErrorKeepsPassword(t *testing.T) {
	path := newDB(t, true)
	errCancelled := errors.New("cancelled")

	err := resetPassword(&bytes.Buffer{}, path, "", func() (string, error) { return "", errCancelled })

	require.ErrorIs(t, err, errCancelled)
	assert.NoError(t, authenticate(t, path, "old-secret"))
}

func TestResetPassword_RejectsShortPassword(t *testing.T) {
	path := newDB(t, true)

	err := resetPassword(&bytes.Buffer{}, path, "12345", noPrompt(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 6")
	assert.NoError(t, authenticate(t, path, "old-secret"))
}

func TestResetPassword_NoAdmin(t *testing.T) {
	path := newDB(t, false)

	err := resetPassword(&bytes.Buffer{}, path, "new-secret", noPrompt(t))

	require.EqualError(t, err, i18n.M.Output.Reset.NoAdmin)
}

func TestResetPassword_MissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "openbee.db")

	err := resetPassword(&bytes.Buffer{}, path, "new-secret", noPrompt(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.NoDirExists(t, filepath.Dir(path))
}
