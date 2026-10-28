package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const errorBodyLimit = 1 << 20

var (
	ErrNotOpenBee   = errors.New("not an OpenBee server")
	ErrUnauthorized = errors.New("unauthorized")
)

type Error struct {
	Status  int
	Message string
	Code    string
	Params  map[string]any
	Details map[string]any
}

func (e *Error) Error() string {
	return e.Message
}

func IsStatus(err error, status int) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == status
}

func mediaType(contentType string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(contentType))
	}
	return mt
}

func isJSONType(mt string) bool {
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

func errorFromResponse(resp *http.Response) *Error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
	e := &Error{Status: resp.StatusCode}
	if isJSONType(mediaType(resp.Header.Get("Content-Type"))) {
		var body map[string]any
		if json.Unmarshal(data, &body) == nil {
			e.Details = body
			e.Message, _ = body["error"].(string)
			e.Code, _ = body["code"].(string)
			e.Params, _ = body["params"].(map[string]any)
		}
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return e
}
