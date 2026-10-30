package api

import (
	"context"
	"net/http"
)

func pluginPath(id string) string {
	return "/plugins/" + escapeSegment(id)
}

func PluginSetupEventsPath(sid string) string {
	return "/plugins/setup/" + escapeSegment(sid) + "/events"
}

func (c *Client) Plugins(ctx context.Context) ([]PluginStatus, error) {
	var out struct {
		Plugins []PluginStatus `json:"plugins"`
	}
	if err := c.Do(ctx, http.MethodGet, "/plugins", nil, &out); err != nil {
		return nil, err
	}
	if out.Plugins == nil {
		out.Plugins = []PluginStatus{}
	}
	return out.Plugins, nil
}

func (c *Client) Plugin(ctx context.Context, id string) (PluginDetail, error) {
	var out PluginDetail
	err := c.Do(ctx, http.MethodGet, pluginPath(id), nil, &out)
	return out, err
}

func (c *Client) PluginCatalog(ctx context.Context, refresh bool) (PluginCatalog, error) {
	path := "/plugins/catalog"
	if refresh {
		path += "?refresh=1"
	}
	var out PluginCatalog
	err := c.Do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) InstallPlugin(ctx context.Context, spec string) (PluginOperation, error) {
	var out PluginOperation
	err := c.Do(ctx, http.MethodPost, "/plugins/install", map[string]string{"spec": spec}, &out)
	return out, err
}

func (c *Client) PluginOperations(ctx context.Context) ([]PluginOperation, error) {
	var out struct {
		Operations []PluginOperation `json:"operations"`
	}
	if err := c.Do(ctx, http.MethodGet, "/plugins/operations", nil, &out); err != nil {
		return nil, err
	}
	if out.Operations == nil {
		out.Operations = []PluginOperation{}
	}
	return out.Operations, nil
}

func (c *Client) UpdatePluginSettings(ctx context.Context, id string, req PluginSettingsUpdate) error {
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	return c.Do(ctx, http.MethodPut, pluginPath(id)+"/settings", req, nil)
}

func (c *Client) EnablePlugin(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, pluginPath(id)+"/enable", nil, nil)
}

func (c *Client) DisablePlugin(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, pluginPath(id)+"/disable", nil, nil)
}

func (c *Client) RestartPlugin(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, pluginPath(id)+"/restart", nil, nil)
}

func (c *Client) UninstallPlugin(ctx context.Context, id string) (PluginOperation, error) {
	var out PluginOperation
	err := c.Do(ctx, http.MethodDelete, pluginPath(id), nil, &out)
	return out, err
}

func (c *Client) StartPluginSetup(ctx context.Context, id string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.Do(ctx, http.MethodPost, pluginPath(id)+"/setup", nil, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (c *Client) PluginSetupInput(ctx context.Context, sid, text string) error {
	return c.Do(ctx, http.MethodPost, "/plugins/setup/"+escapeSegment(sid)+"/input", map[string]string{"text": text}, nil)
}

func (c *Client) CancelPluginSetup(ctx context.Context, sid string) error {
	return c.Do(ctx, http.MethodDelete, "/plugins/setup/"+escapeSegment(sid), nil, nil)
}
