package api

import (
	"context"
	"net/http"
)

func workerPath(id string) string {
	return "/workers/" + escapeSegment(id)
}

func (c *Client) Workers(ctx context.Context) ([]Worker, error) {
	var out []Worker
	if err := c.Do(ctx, http.MethodGet, "/workers", nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Worker{}
	}
	return out, nil
}

func (c *Client) Worker(ctx context.Context, id string) (Worker, error) {
	var out Worker
	err := c.Do(ctx, http.MethodGet, workerPath(id), nil, &out)
	return out, err
}

func (c *Client) CreateWorker(ctx context.Context, req CreateWorkerRequest) (Worker, error) {
	var out Worker
	err := c.Do(ctx, http.MethodPost, "/workers", req, &out)
	return out, err
}

func (c *Client) UpdateWorker(ctx context.Context, id string, req UpdateWorkerRequest) (Worker, error) {
	var out Worker
	err := c.Do(ctx, http.MethodPut, workerPath(id), req, &out)
	return out, err
}

func (c *Client) DeleteWorker(ctx context.Context, id string, deleteWorkDir bool) error {
	path := workerPath(id)
	if deleteWorkDir {
		path += "?delete_work_dir=true"
	}
	return c.Do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) RandomWorkerName(ctx context.Context) (RandomName, error) {
	var out RandomName
	err := c.Do(ctx, http.MethodGet, "/workers/random-name", nil, &out)
	return out, err
}
