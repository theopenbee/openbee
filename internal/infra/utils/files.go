package utils

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}

func MoveFile(src, dst string) error {
	renameErr := os.Rename(src, dst)
	if renameErr == nil {
		return nil
	}
	if err := CopyFile(src, dst); err != nil {
		return errors.Join(renameErr, err)
	}
	return os.Remove(src)
}
