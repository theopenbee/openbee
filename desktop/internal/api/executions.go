package api

import (
	"context"
	"net/http"
)

func (c *Client) Execution(ctx context.Context, id string) (ExecutionDetail, error) {
	var out ExecutionDetail
	err := c.Do(ctx, http.MethodGet, "/executions/"+escapeSegment(id), nil, &out)
	return out, err
}
