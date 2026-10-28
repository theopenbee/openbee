package api

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Client) Envs(ctx context.Context, scope, scopeID string) ([]EnvConfig, error) {
	query := url.Values{"scope": {scope}}
	if scopeID != "" {
		query.Set("scope_id", scopeID)
	}
	var out []EnvConfig
	if err := c.Do(ctx, http.MethodGet, "/envs?"+query.Encode(), nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []EnvConfig{}
	}
	return out, nil
}

func (c *Client) CreateEnv(ctx context.Context, req CreateEnvRequest) (EnvConfig, error) {
	var out EnvConfig
	err := c.Do(ctx, http.MethodPost, "/envs", req, &out)
	return out, err
}

func (c *Client) UpdateEnv(ctx context.Context, id, value string) error {
	return c.Do(ctx, http.MethodPut, "/envs/"+escapeSegment(id), map[string]string{"value": value}, nil)
}

func (c *Client) DeleteEnv(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/envs/"+escapeSegment(id), nil, nil)
}
