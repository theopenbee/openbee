package api

import (
	"context"
	"net/http"
)

type ServerVersion struct {
	Version string `json:"version"`
	Bundled bool   `json:"bundled"`
}

func (c *Client) Version(ctx context.Context) (ServerVersion, error) {
	var out ServerVersion
	err := c.Do(ctx, http.MethodGet, "/version", nil, &out)
	return out, err
}
