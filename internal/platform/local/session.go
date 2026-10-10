package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/theopenbee/openbee/internal/infra/utils"
)

const sessionKeyPrefix = PlatformID + ":"

const legacySessionKey = sessionKeyPrefix + "default"

type SessionClaimStore interface {
	SessionKeyClaim(ctx context.Context, from string) (string, bool, error)
	ClaimSessionKey(ctx context.Context, from, to string) (string, error)
}

type LegacySession struct {
	store     SessionClaimStore
	mediaRoot string
	owner     atomic.Pointer[string]
}

func NewLegacySession(store SessionClaimStore, mediaRoot string) *LegacySession {
	return &LegacySession{store: store, mediaRoot: mediaRoot}
}

func SessionKey(userID string) string {
	return sessionKeyPrefix + userID
}

func MediaDir(root, sessionKey string) (string, error) {
	id, ok := strings.CutPrefix(sessionKey, sessionKeyPrefix)
	if !ok || id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid local session key %q", sessionKey)
	}
	return filepath.Join(root, id), nil
}

func (l *LegacySession) Restore(ctx context.Context, findOwner func() (string, bool, error)) error {
	owner, claimed, err := l.store.SessionKeyClaim(ctx, legacySessionKey)
	if err != nil {
		return fmt.Errorf("load legacy session claim: %w", err)
	}
	if claimed {
		l.owner.Store(&owner)
		return l.moveMedia(owner)
	}
	userID, found, err := findOwner()
	if err != nil {
		return fmt.Errorf("find legacy session owner: %w", err)
	}
	if !found {
		return nil
	}
	return l.Claim(ctx, userID)
}

func (l *LegacySession) Claim(ctx context.Context, userID string) error {
	sessionKey := SessionKey(userID)
	if sessionKey == legacySessionKey {
		return nil
	}
	owner, err := l.store.ClaimSessionKey(ctx, legacySessionKey, sessionKey)
	if err != nil {
		return fmt.Errorf("claim legacy session: %w", err)
	}
	l.owner.Store(&owner)
	return l.moveMedia(owner)
}

func (l *LegacySession) Resolve(sessionKey string) string {
	if l == nil || sessionKey != legacySessionKey {
		return sessionKey
	}
	if owner := l.owner.Load(); owner != nil {
		return *owner
	}
	return sessionKey
}

func (l *LegacySession) moveMedia(owner string) error {
	dst, err := MediaDir(l.mediaRoot, owner)
	if err != nil {
		return err
	}
	src, err := MediaDir(l.mediaRoot, legacySessionKey)
	if err != nil {
		return err
	}
	if err := moveDirContents(src, dst); err != nil {
		return fmt.Errorf("move legacy media: %w", err)
	}
	return nil
}

func moveDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if err := moveFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := os.Remove(src); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func moveFile(src, dst string) error {
	renameErr := os.Rename(src, dst)
	if renameErr == nil || errors.Is(renameErr, fs.ErrNotExist) {
		return nil
	}
	if err := utils.CopyFile(src, dst); err != nil {
		return errors.Join(renameErr, err)
	}
	return os.Remove(src)
}
