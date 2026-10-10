package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
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
}

func NewLegacySession(store SessionClaimStore, mediaRoot string) *LegacySession {
	return &LegacySession{store: store, mediaRoot: mediaRoot}
}

func SessionKey(userID string) string {
	return sessionKeyPrefix + userID
}

func MediaDir(root, sessionKey string) (string, error) {
	if root == "" {
		return "", errors.New("local media root is unavailable")
	}
	id, ok := strings.CutPrefix(sessionKey, sessionKeyPrefix)
	if !ok || id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid local session key %q", sessionKey)
	}
	return filepath.Join(root, id), nil
}

func MediaFileName(messageID, mediaPath string) string {
	return messageID + "_" + platform.SanitizeFileName(filepath.Base(mediaPath))
}

func (l *LegacySession) Restore(ctx context.Context, findOwner func() (string, bool, error)) error {
	owner, claimed, err := l.store.SessionKeyClaim(ctx, legacySessionKey)
	if err != nil {
		return fmt.Errorf("load legacy session claim: %w", err)
	}
	if !claimed {
		userID, found, err := findOwner()
		if err != nil {
			return fmt.Errorf("find legacy session owner: %w", err)
		}
		if !found {
			return nil
		}
		owner = SessionKey(userID)
	}
	return l.claim(ctx, owner)
}

func (l *LegacySession) Claim(ctx context.Context, userID string) error {
	return l.claim(ctx, SessionKey(userID))
}

func (l *LegacySession) claim(ctx context.Context, sessionKey string) error {
	if sessionKey == legacySessionKey {
		return nil
	}
	owner, err := l.store.ClaimSessionKey(ctx, legacySessionKey, sessionKey)
	if err != nil {
		return fmt.Errorf("claim legacy session: %w", err)
	}
	return l.moveMedia(owner)
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
		err := utils.MoveFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
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
