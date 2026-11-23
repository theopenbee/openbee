package api

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Client) EngineModels(ctx context.Context, engine, workerID string) ([]EngineModel, error) {
	path := "/engines/" + escapeSegment(engine) + "/models"
	if workerID != "" {
		path += "?" + url.Values{"worker_id": {workerID}}.Encode()
	}
	var out struct {
		Models []EngineModel `json:"models"`
	}
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	if out.Models == nil {
		out.Models = []EngineModel{}
	}
	return out.Models, nil
}
