package api

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
	"sync"
	"time"
)

const (
	apiPrefix      = "/api"
	defaultTimeout = 30 * time.Second
)

type TokenSource interface {
	Access() string
	Refresh() string
	SetAccess(token string)
}

type Options struct {
	BaseURL        string
	HTTP           *http.Client
	Tokens         TokenSource
	Language       func() string
	OnUnauthorized func()
}

type Client struct {
	baseURL        string
	http           *http.Client
	tokens         TokenSource
	language       func() string
	onUnauthorized func()

	mu       sync.Mutex
	flight   *flight
	rejected string
}

type noTokens struct{}

func (noTokens) Access() string   { return "" }
func (noTokens) Refresh() string  { return "" }
func (noTokens) SetAccess(string) {}

var defaultHTTPClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 120 * time.Second,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          10,
	IdleConnTimeout:       90 * time.Second,
}}

func New(o Options) *Client {
	hc := o.HTTP
	if hc == nil {
		hc = defaultHTTPClient
	}
	tokens := o.Tokens
	if tokens == nil {
		tokens = noTokens{}
	}
	return &Client{
		baseURL:        strings.TrimRight(o.BaseURL, "/"),
		http:           hc,
		tokens:         tokens,
		language:       o.Language,
		onUnauthorized: o.OnUnauthorized,
	}
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func isPublic(path string) bool {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	switch path {
	case "/auth/login", "/auth/refresh", "/setup", "/setup/status", "/config":
		return true
	}
	return false
}

func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+apiPrefix+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.send(req, !isPublic(path))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, out)
}

func (c *Client) DoRaw(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.send(req.WithContext(ctx), true)
}

func decodeResponse(resp *http.Response, out any) error {
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	mt := mediaType(resp.Header.Get("Content-Type"))
	if mt == "text/html" {
		return ErrNotOpenBee
	}
	if out == nil {
		return nil
	}
	if !isJSONType(mt) {
		return ErrNotOpenBee
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) send(req *http.Request, auth bool) (*http.Response, error) {
	resp, used, err := c.attempt(req, auth)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && auth {
		retry, ok := replayable(req)
		if !ok {
			return nil, errorFromResponse(resp)
		}
		discard(resp)
		if err := c.refresh(req.Context(), used); err != nil {
			return nil, err
		}
		resp, used, err = c.attempt(retry, true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized {
			discard(resp)
			c.reject(tokenKey(used, c.tokens.Refresh()))
			return nil, ErrUnauthorized
		}
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, errorFromResponse(resp)
	}
	return resp, nil
}

func (c *Client) attempt(req *http.Request, auth bool) (*http.Response, string, error) {
	r := req.Clone(req.Context())
	token := ""
	if auth {
		token = c.tokens.Access()
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if c.language != nil {
			if lang := c.language(); lang != "" {
				r.Header.Set("Accept-Language", lang)
			}
		}
	} else {
		r.Header.Del("Authorization")
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return nil, "", err
	}
	return resp, token, nil
}

func replayable(req *http.Request) (*http.Request, bool) {
	retry := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return retry, true
	}
	if req.GetBody == nil {
		return nil, false
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}
	retry.Body = body
	return retry, true
}

func discard(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
}

func NormalizeBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("server address is empty")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid server address: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return "", errors.New("server address has no host")
	}
	p := strings.TrimRight(u.EscapedPath(), "/")
	p = strings.TrimSuffix(p, "/api")
	p = strings.TrimRight(p, "/")
	return u.Scheme + "://" + u.Host + p, nil
}

func Probe(ctx context.Context, hc *http.Client, baseURL string) (Config, error) {
	c := New(Options{BaseURL: baseURL, HTTP: hc})
	var cfg Config
	if err := c.Do(ctx, http.MethodGet, "/config", nil, &cfg); err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
			return Config{}, ErrNotOpenBee
		}
		return Config{}, err
	}
	return cfg, nil
}
