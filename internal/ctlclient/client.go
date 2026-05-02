// Package ctlclient provides an HTTP client for the openbee /rpc/bee/call endpoint,
// used by the openbee ctl CLI subcommands.
package ctlclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
)

const defaultURL = "http://localhost:8080"

// Client calls the openbee /rpc/bee/call endpoint.
// Construct via NewClient or directly (useful in tests).
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient resolves connection config in priority order:
//  1. OPENBEE_URL / OPENBEE_API_KEY environment variables
//  2. config file at cfgPath: derives base URL and generates a bee JWT from token_secret
//  3. defaults: http://localhost:8080, empty API key
func NewClient(cfgPath string) (*Client, error) {
	baseURL := os.Getenv("OPENBEE_URL")
	apiKey := os.Getenv("OPENBEE_API_KEY")

	if baseURL == "" || apiKey == "" {
		cfg, err := config.Load(cfgPath)
		if err == nil {
			if baseURL == "" {
				baseURL = cfg.Bee.RPCBaseURL
			}
			if apiKey == "" && cfg.Bee.RPC.TokenSecret != "" {
				token, err := auth.GenerateBeeToken(cfg.Bee.RPC.TokenSecret, cfg.Bee.RPC.TokenTTL)
				if err == nil {
					apiKey = token
				}
			}
		}
	}

	if baseURL == "" {
		baseURL = defaultURL
	}

	return &Client{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type callRequest struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

type callResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Call invokes a named RPC tool and returns the raw JSON result.
// Returns an error for connection failures, auth errors, and tool execution errors.
func (c *Client) Call(toolName string, args any) (json.RawMessage, error) {
	body, err := json.Marshal(callRequest{Name: toolName, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+config.RPCBeeBasePath+"/call", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to openbee server at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized – check OPENBEE_API_KEY")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	var cr callResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if cr.Error != "" {
		return nil, errors.New(cr.Error)
	}
	return cr.Result, nil
}
