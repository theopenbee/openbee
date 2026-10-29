package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	uploadTimeout   = 10 * time.Minute
	downloadTimeout = 5 * time.Minute
	mediaCacheDir   = "media"
)

var errNoCacheDir = errors.New("media cache directory is not set")

type mediaFetch struct {
	waiters []func(localPath string, err error)
}

func (s *Store) initMedia() {
	s.media = map[string]*mediaFetch{}
	s.resets = append(s.resets, func() { s.media = map[string]*mediaFetch{} })
}

func (s *Store) Upload(paths []string, done func(ok []string, failed int)) {
	if done == nil {
		done = func([]string, int) {}
	}
	paths = slices.Clone(paths)
	s.async(func(ctx context.Context, c *api.Client) func() {
		results := make([]string, len(paths))
		var wg sync.WaitGroup
		for i, p := range paths {
			wg.Go(func() { results[i], _ = uploadFile(ctx, c, p) })
		}
		wg.Wait()
		var ok []string
		for _, r := range results {
			if r != "" {
				ok = append(ok, r)
			}
		}
		return func() { done(ok, len(paths)-len(ok)) }
	}, func() { done(nil, len(paths)) })
}

func (s *Store) UploadBytes(name string, data []byte, done func(path string, err error)) {
	if done == nil {
		done = func(string, error) {}
	}
	s.async(func(ctx context.Context, c *api.Client) func() {
		path, err := upload(ctx, c, name, bytes.NewReader(data))
		return func() { done(path, err) }
	}, func() { done("", context.Canceled) })
}

func (s *Store) MediaFile(mediaPath string, done func(localPath string, err error)) {
	if done == nil {
		done = func(string, error) {}
	}
	name, err := mediaName(mediaPath)
	if err == nil && s.o.CacheDir == "" {
		err = errNoCacheDir
	}
	if err != nil {
		s.o.Post(func() { done("", err) })
		return
	}
	if f, ok := s.media[name]; ok {
		f.waiters = append(f.waiters, done)
		return
	}
	f := &mediaFetch{waiters: []func(string, error){done}}
	s.media[name] = f
	finish := func(localPath string, err error) {
		if s.media[name] == f {
			delete(s.media, name)
		}
		for _, w := range f.waiters {
			w(localPath, err)
		}
	}
	dir := filepath.Join(s.o.CacheDir, mediaCacheDir)
	s.async(func(ctx context.Context, c *api.Client) func() {
		localPath, err := cacheMedia(ctx, c, dir, name)
		return func() { finish(localPath, err) }
	}, func() { finish("", context.Canceled) })
}

func uploadFile(ctx context.Context, c *api.Client, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return upload(ctx, c, filepath.Base(path), f)
}

func upload(ctx context.Context, c *api.Client, name string, r io.Reader) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	return c.UploadMedia(ctx, name, r)
}

func cacheMedia(ctx context.Context, c *api.Client, dir, name string) (string, error) {
	localPath := filepath.Join(dir, name)
	if info, err := os.Stat(localPath); err == nil && info.Mode().IsRegular() {
		return localPath, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", wrap("create media cache", err)
	}
	tmp, err := os.CreateTemp(dir, "download-*.part")
	if err != nil {
		return "", wrap("create media cache", err)
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	_, err = c.DownloadMedia(ctx, name, tmp)
	if closeErr := wrap("write media cache", tmp.Close()); err == nil {
		err = closeErr
	}
	if err == nil {
		if tagErr := markDownloaded(tmp.Name()); tagErr != nil {
			log.Printf("store: mark %s as downloaded: %v", name, tagErr)
		}
		err = wrap("write media cache", os.Rename(tmp.Name(), localPath))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return localPath, nil
}

func mediaName(mediaPath string) (string, error) {
	name := mediaPath
	if i := strings.LastIndexAny(mediaPath, `/\`); i >= 0 {
		name = mediaPath[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid media path %q", mediaPath)
	}
	return name, nil
}
