package api

import (
	"context"
	"net/http"
)

func (c *Client) SystemConfigs(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	if err := c.Do(ctx, http.MethodGet, "/system-configs", nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}

func (c *Client) SetSystemConfig(ctx context.Context, key, value string) error {
	return c.Do(ctx, http.MethodPut, "/system-configs/"+escapeSegment(key), map[string]string{"value": value}, nil)
}
