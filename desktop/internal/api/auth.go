package api

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const refreshTimeout = 30 * time.Second

type flight struct {
	done chan struct{}
	err  error
}

func tokenKey(access, refresh string) string {
	return access + "\x00" + refresh
}

func (c *Client) EnsureFresh(ctx context.Context) error {
	if !needsRefresh(c.tokens.Access(), time.Now()) {
		return nil
	}
	return c.RefreshNow(ctx)
}

func (c *Client) RefreshNow(ctx context.Context) error {
	return c.refresh(ctx, c.tokens.Access())
}

func (c *Client) refresh(ctx context.Context, used string) error {
	c.mu.Lock()
	if current := c.tokens.Access(); current != used {
		c.mu.Unlock()
		if current == "" {
			return ErrUnauthorized
		}
		return nil
	}
	refreshToken := c.tokens.Refresh()
	key := tokenKey(used, refreshToken)
	if c.rejected == key {
		c.mu.Unlock()
		return ErrUnauthorized
	}
	f := c.flight
	if f == nil {
		f = &flight{done: make(chan struct{})}
		c.flight = f
		go c.runRefresh(f, key, refreshToken)
	}
	c.mu.Unlock()

	select {
	case <-f.done:
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) runRefresh(f *flight, key, refreshToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	err := c.requestRefresh(ctx, refreshToken)
	cancel()

	c.mu.Lock()
	c.flight = nil
	c.mu.Unlock()

	if errors.Is(err, ErrUnauthorized) {
		c.reject(key)
	}
	f.err = err
	close(f.done)
}

func (c *Client) reject(key string) {
	c.mu.Lock()
	first := c.rejected != key
	c.rejected = key
	c.mu.Unlock()
	if first && c.onUnauthorized != nil {
		c.onUnauthorized()
	}
}

func (c *Client) requestRefresh(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return ErrUnauthorized
	}
	var pair TokenPair
	err := c.Do(ctx, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": refreshToken}, &pair)
	if err != nil {
		if IsStatus(err, http.StatusUnauthorized) {
			return ErrUnauthorized
		}
		return err
	}
	if pair.AccessToken == "" {
		return ErrNotOpenBee
	}
	c.tokens.SetAccess(pair.AccessToken)
	return nil
}
