package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const MaxAvatarImage int64 = 1 << 20

var (
	ErrInvalidAvatar  = errors.New("invalid avatar reference")
	ErrAvatarTooLarge = errors.New("avatar image is larger than the limit")
)

func SplitAvatar(ref string) (plugin, name string, ok bool) {
	plugin, name, found := strings.Cut(ref, "/")
	if !found || !avatarSegment(plugin) || !avatarSegment(name) {
		return "", "", false
	}
	return plugin, name, true
}

func avatarSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}

func (c *Client) Avatars(ctx context.Context) ([]AvatarPack, error) {
	var out struct {
		Packs []AvatarPack `json:"packs"`
	}
	if err := c.Do(ctx, http.MethodGet, "/avatars", nil, &out); err != nil {
		return nil, err
	}
	packs := make([]AvatarPack, 0, len(out.Packs))
	for _, p := range out.Packs {
		if p.Avatars == nil {
			p.Avatars = []string{}
		}
		packs = append(packs, p)
	}
	return packs, nil
}

func (c *Client) AvatarImage(ctx context.Context, ref string) ([]byte, error) {
	plugin, name, ok := SplitAvatar(ref)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAvatar, ref)
	}
	path := c.baseURL + apiPrefix + "/avatars/" + escapeSegment(plugin) + "/" + escapeSegment(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/*")
	resp, err := c.DoRaw(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > MaxAvatarImage {
		return nil, ErrAvatarTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxAvatarImage+1))
	if err != nil {
		return nil, fmt.Errorf("read avatar %q: %w", ref, err)
	}
	if int64(len(data)) > MaxAvatarImage {
		return nil, ErrAvatarTooLarge
	}
	return data, nil
}
