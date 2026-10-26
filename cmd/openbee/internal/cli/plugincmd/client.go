package plugincmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
)

const adminTokenTTL = 5 * time.Minute

var (
	ErrNotRunning       = errors.New("openbee is not running")
	errAgentEnvironment = errors.New("plugin management is not available inside openbee agents")
	errNoTokenSecret    = errors.New("bee.rpc.token_secret is not set in config.yaml")
)

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

type Plugin struct {
	ID          string               `json:"id"`
	Package     string               `json:"package"`
	Source      string               `json:"source"`
	Version     string               `json:"version"`
	Path        string               `json:"path"`
	Name        plugin.LocalizedText `json:"name"`
	Description plugin.LocalizedText `json:"description"`
	Enabled     bool                 `json:"enabled"`
	BotName     string               `json:"bot_name"`
	State       string               `json:"state"`
	Error       string               `json:"error"`
	HasSetup    bool                 `json:"has_setup"`
}

type Detail struct {
	Plugin
	Config     map[string]any       `json:"config"`
	Fields     []plugin.ConfigField `json:"fields"`
	SetupLabel plugin.LocalizedText `json:"setup_label"`
}

type CatalogPlugin struct {
	catalog.Entry
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	UpdateAvailable  bool   `json:"update_available"`
	Installable      bool   `json:"installable"`
}

type Catalog struct {
	SchemaVersion int             `json:"schema_version"`
	Categories    []catalog.Vocab `json:"categories"`
	Tags          []catalog.Vocab `json:"tags"`
	Plugins       []CatalogPlugin `json:"plugins"`
}

type Operation struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Spec     string `json:"spec"`
	PluginID string `json:"plugin_id"`
	Status   string `json:"status"`
	Error    string `json:"error"`
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClientFromConfig(cfgPath string, getenv func(string) string) (*Client, error) {
	if getenv("OPENBEE_API_KEY") != "" {
		return nil, errAgentEnvironment
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", cfgPath, err)
	}
	if cfg.Bee.RPC.TokenSecretGenerated {
		return nil, errNoTokenSecret
	}
	token, err := auth.GenerateAdminToken(cfg.Bee.RPC.TokenSecret, adminTokenTTL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: strings.TrimSuffix(cfg.Bee.RPCBaseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return fmt.Errorf("%w at %s", ErrNotRunning, c.baseURL)
		}
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("%s %s: HTTP %d", method, path, resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Code: e.Code, Message: e.Error}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) List(ctx context.Context) ([]Plugin, error) {
	var out struct {
		Plugins []Plugin `json:"plugins"`
	}
	err := c.do(ctx, http.MethodGet, "/api/plugins", nil, &out)
	return out.Plugins, err
}

func (c *Client) Get(ctx context.Context, id string) (Detail, error) {
	var d Detail
	err := c.do(ctx, http.MethodGet, "/api/plugins/"+url.PathEscape(id), nil, &d)
	return d, err
}

func (c *Client) Catalog(ctx context.Context, refresh bool) (*Catalog, error) {
	path := "/api/plugins/catalog"
	if refresh {
		path += "?refresh=1"
	}
	var cat Catalog
	if err := c.do(ctx, http.MethodGet, path, nil, &cat); err != nil {
		return nil, err
	}
	return &cat, nil
}

func (c *Client) Install(ctx context.Context, spec string) (Operation, error) {
	var op Operation
	err := c.do(ctx, http.MethodPost, "/api/plugins/install", map[string]string{"spec": spec}, &op)
	return op, err
}

func (c *Client) Operation(ctx context.Context, id string) (Operation, error) {
	var op Operation
	err := c.do(ctx, http.MethodGet, "/api/plugins/operations/"+url.PathEscape(id), nil, &op)
	return op, err
}

func (c *Client) UpdateSettings(ctx context.Context, id string, botName *string, cfg map[string]any) error {
	body := map[string]any{"config": cfg}
	if cfg == nil {
		body["config"] = map[string]any{}
	}
	if botName != nil {
		body["bot_name"] = *botName
	}
	return c.do(ctx, http.MethodPut, "/api/plugins/"+url.PathEscape(id)+"/settings", body, nil)
}

func (c *Client) Enable(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/plugins/"+url.PathEscape(id)+"/enable", nil, nil)
}

func (c *Client) Disable(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/plugins/"+url.PathEscape(id)+"/disable", nil, nil)
}

func (c *Client) Restart(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/plugins/"+url.PathEscape(id)+"/restart", nil, nil)
}

func (c *Client) Uninstall(ctx context.Context, id string) (Operation, error) {
	var op Operation
	err := c.do(ctx, http.MethodDelete, "/api/plugins/"+url.PathEscape(id), nil, &op)
	return op, err
}
