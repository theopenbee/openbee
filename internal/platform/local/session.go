package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/theopenbee/openbee/internal/platform"
)

const sessionKeyPrefix = PlatformID + ":"

const legacySessionKey = sessionKeyPrefix + "default"

type SessionKeyReassigner interface {
	ReassignSessionKey(ctx context.Context, from, to string) error
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

func MediaFileName(messageID, mediaPath string) string {
	return messageID + "_" + platform.SanitizeFileName(filepath.Base(mediaPath))
}

func ClaimLegacySession(ctx context.Context, sessions SessionKeyReassigner, mediaRoot, userID string) error {
	sessionKey := SessionKey(userID)
	if sessionKey == legacySessionKey {
		return nil
	}
	dir, err := MediaDir(mediaRoot, sessionKey)
	if err != nil {
		return err
	}
	legacyDir, err := MediaDir(mediaRoot, legacySessionKey)
	if err != nil {
		return err
	}
	if err := moveDirContents(legacyDir, dir); err != nil {
		return fmt.Errorf("move legacy media: %w", err)
	}
	if err := sessions.ReassignSessionKey(ctx, legacySessionKey, sessionKey); err != nil {
		return fmt.Errorf("reassign legacy session: %w", err)
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
	for _, e := range entries {
		if err := os.Rename(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
