package api

import (
	"context"
	"net/http"
)

func (c *Client) Config(ctx context.Context) (Config, error) {
	var out Config
	err := c.Do(ctx, http.MethodGet, "/config", nil, &out)
	return out, err
}

func (c *Client) SetupStatus(ctx context.Context) (SetupStatus, error) {
	var out SetupStatus
	err := c.Do(ctx, http.MethodGet, "/setup/status", nil, &out)
	return out, err
}

func (c *Client) Setup(ctx context.Context, req SetupRequest) (TokenPair, error) {
	var out TokenPair
	err := c.Do(ctx, http.MethodPost, "/setup", req, &out)
	return out, err
}

func (c *Client) Login(ctx context.Context, username, password string) (TokenPair, error) {
	var out TokenPair
	body := map[string]string{"username": username, "password": password}
	err := c.Do(ctx, http.MethodPost, "/auth/login", body, &out)
	return out, err
}

func (c *Client) ChangePassword(ctx context.Context, oldPassword, newPassword string) error {
	body := map[string]string{"old_password": oldPassword, "new_password": newPassword}
	return c.Do(ctx, http.MethodPost, "/me/password", body, nil)
}
