# Linear Attachments Design

Status: Approved (brainstorming)
Date: 2026-05-04
Owners: openbee linear platform

## Background

Linear issues and comments embed media via authenticated URLs on
`uploads.linear.app`. The current Linear platform adapter
(`internal/platform/linear/handler.go`) passes raw markdown straight to the
LLM, so the model receives URLs it cannot fetch. The outbound `Sender` also
rejects any `OutboundMessage.MediaPath` with `"linear: media attachments not
supported in v0"`.

Feishu and Telegram already implement the inbound download / outbound upload
pattern through `internal/infra/media/Service`, producing `<media:image
path="..." name="...">` placeholders that the LLM understands. Linear should
join the same pattern, with platform-specific download authentication and a
two-step upload via the `fileUpload` GraphQL mutation.

## Goals

1. **Inbound:** Detect every markdown image / link pointing at
   `uploads.linear.app` inside `Issue.Description` and `Comment.Body`,
   download the asset (authenticated with the workspace API key), persist via
   `media.Service.SaveInbound`, and replace the markdown segment with a
   `<media:*>` placeholder so the LLM can read the file locally.
2. **Outbound:** When a worker reply carries `OutboundMessage.MediaPath`,
   upload the file to Linear (`fileUpload` mutation + presigned PUT) and
   append the resulting markdown image / link to the comment body so a single
   comment delivers text and media together.
3. Reuse the existing `media.Service`, `BuildPlaceholder`, and `MaxMediaSize`
   conventions to stay consistent with feishu / telegram.

## Non-Goals

- Bare-URL detection (only markdown image / link forms).
- HTML `<img>` parsing (Linear's web client never emits this in markdown).
- Multi-file outbound (`MediaPath` stays a single path; YAGNI).
- Retroactive backfill of attachments in already-seen issues.

## User Decisions

Captured during brainstorming on 2026-05-04:

| Question | Decision |
|----------|----------|
| Asset scope | All `uploads.linear.app` resources (image / video / audio / document), MIME-routed. |
| Replacement strategy | Replace the full markdown segment with `<media:...>` placeholder; original URL is dropped on success. |
| Outbound shape | Upload, then append markdown to `Content`; single comment with mixed text + media. |
| Failure fallback | Inline placeholder without `path` plus an `Original: <url>` line so the LLM can still cite the source. |
| Default `MaxMediaSize` | 50 MB, aligned with Telegram. |

## Architecture Overview

```
internal/platform/linear/
  client.go         // adds DownloadAsset / FileUpload to the Client interface
  handler.go        // orchestration only; calls resolver / uploader
  attachments.go    // new: extractor + resolver + uploader
  attachments_test.go
```

### Inbound flow

```
poll → IssuesInStates → for each Issue.Description and Comment.Body:
  resolver.Resolve(ctx, text) string
    1. extractAssetURLs(text) → []assetMatch (with span, url, alt, isImage)
    2. concurrent (errgroup, max 4) for each match:
         a. client.DownloadAsset(ctx, url) using API key auth
         b. mediaSvc.SaveInbound(data, ext)
         c. mediaSvc.BuildPlaceholder(mediaType, path, alt)
       failure → "<media:image name=\"alt\">\nOriginal: <url>"
    3. splice replacements into the original text in reverse span order
→ buildInitialInbound / buildCommentInbound use the resolved text
```

### Outbound flow

```
Sender.Send(msg):
  if msg.MediaPath != "":
    md := uploader.Upload(ctx, msg.MediaPath)
       1. read file, detect MIME, enforce MaxMediaSize
       2. client.FileUpload(name, mime, size) → FileUploadTicket
       3. HTTP PUT bytes to ticket.UploadURL with ticket.Headers
       4. return "![name](assetUrl)" if image else "[name](assetUrl)"
    body := selfMarker + msg.Content + "\n\n" + md
  else:
    body := selfMarker + msg.Content
  client.CreateComment(IssueID, body, ParentCommentID)
```

A single comment carries both the worker's reply text and the uploaded
attachment.

## Interfaces

### Client (client.go)

```go
type FileUploadTicket struct {
    AssetURL  string            // appended into comment markdown
    UploadURL string            // PUT target (S3 presigned)
    Headers   map[string]string // headers required by the presigned PUT
}

type Client interface {
    Viewer(ctx context.Context) (User, error)
    IssuesInStates(ctx context.Context, states []string, label string, projects []string) ([]Issue, error)
    CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error)

    // DownloadAsset fetches uploads.linear.app bytes using the workspace
    // API key in the Authorization header. Returns the body and the
    // server-reported Content-Type.
    DownloadAsset(ctx context.Context, url string) (data []byte, contentType string, err error)

    // FileUpload runs Linear's fileUpload mutation and returns the
    // presigned upload target plus the asset URL to embed.
    FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error)
}
```

The S3 PUT is intentionally not part of `Client` — `Client` stays a thin
GraphQL/HTTP layer. The PUT lives in `uploader`.

### Resolver / Uploader (attachments.go)

```go
type assetMatch struct {
    span      [2]int  // byte offsets in the original text
    url       string  // https://uploads.linear.app/...
    altOrName string  // markdown alt or link text; may be empty
    isImage   bool    // true for ![..](..)
}

func extractAssetURLs(text string) []assetMatch

type resolver struct {
    client  Client
    media   *media.Service
    maxSize int
}

// Resolve never returns an error. Per-asset failures fall back to the
// placeholder + Original-URL pattern; unrelated content passes through.
func (r *resolver) Resolve(ctx context.Context, text string) string

type uploader struct {
    client  Client
    maxSize int
    http    *http.Client
}

// Upload returns the markdown fragment to append to a comment.
func (u *uploader) Upload(ctx context.Context, path string) (markdown string, err error)
```

### Handler wiring

```go
type LinearReceiver struct {
    // ...existing fields
    resolver *resolver
}

type LinearSender struct {
    client   Client
    uploader *uploader
}
```

`NewPlatform` accepts an additional `*media.Service`, matching the
feishu / telegram constructor shape.

### Configuration

```go
type LinearConfig struct {
    // ...existing fields
    MaxMediaSize int `yaml:"max_media_size"` // default 50 MB
}
```

`applyDefaults`: `if cfg.Linear.MaxMediaSize == 0 { cfg.Linear.MaxMediaSize = 50 * 1024 * 1024 }`.

## Error Handling and Edge Cases

### Inbound fallback (decision: option A from Q4)

| Condition | Outcome |
|-----------|---------|
| HTTP 401 / 403 | warn log; replacement = `<media:TYPE name="alt">\nOriginal: <url>` |
| HTTP 5xx / timeout | same as above; the asset is **not** retried because the SeenSet has already recorded the issue / comment after dispatch (a manual re-comment is required to retry) |
| File exceeds `MaxMediaSize` | warn log; same fallback |
| Non-`uploads.linear.app` URL | left untouched (user-provided external links pass through) |
| URL inside fenced or inline code | left untouched |

`TYPE` in the fallback is `image` when the markdown form was `![..](..)` and
`document` otherwise — the bytes are unavailable so MIME-based routing is
not possible. On the **success** path, the media type is derived from the
downloaded `Content-Type` via `media.MediaTypeFromMIME` regardless of which
markdown form was used.

Concurrency: errgroup with a worker limit of 4 per text body. Cross-issue
parallelism is unchanged (the existing `tickOnce` loop stays sequential).

### Outbound failures

`uploader.Upload` returns an error → `Sender.Send` returns the error → the
existing `RetryWithBackoff` wrapper retries up to `DefaultRetryCount`. A file
larger than `MaxMediaSize` is rejected before the GraphQL call. Missing /
unreadable `MediaPath` files surface a wrapped filesystem error.

### URL extraction robustness

Two markdown forms are recognized:

- `!\[(?P<alt>[^\]]*)\]\((?P<url>https://uploads\.linear\.app/[^)\s]+)\)`
- `\[(?P<text>[^\]]*)\]\((?P<url>https://uploads\.linear\.app/[^)\s]+)\)`

Code regions are masked before extraction:

1. Replace fenced blocks (```` ```...``` ````) and inline code (`` `...` ``)
   with placeholder tokens of the same length.
2. Run regex extraction on the masked text.
3. Restore the original code regions before returning the rewritten text.

This avoids rewriting tutorial / example snippets that happen to contain
upload URLs.

### Placeholder alt escaping

`media.Service.BuildPlaceholder` already wraps `name=` values with `%q`,
which handles quotes, backslashes, and Unicode. No new escaping is needed
in `attachments.go`.

### Timeouts

| Operation | Timeout |
|-----------|---------|
| `DownloadAsset` | 30s |
| `FileUpload` mutation | 30s |
| S3 PUT | 120s (matches feishu file upload) |

All timeouts are derived contexts of the inbound `ctx`, so shutdown still
unwinds promptly.

### Logging

- `DEBUG`: each URL match, download start, download bytes
- `WARN`: per-asset failures with URL and reason
- `ERROR`: only when a batch fails entirely (sanity signal)

## Testing Strategy

### `attachments_test.go` (new)

`extractAssetURLs`:

- single image markdown
- multiple images mixed with prose
- regular link form `[name](url)`
- non-`uploads.linear.app` host (skipped)
- URL inside fenced code (skipped)
- URL inside inline code (skipped)
- alt with Unicode, spaces, quotes, escaped `]`
- escaped parentheses `\(` `\)` boundaries

`resolver.Resolve` (Client is a fake; `media.Service` uses a real temp dir):

- zero matches: text unchanged
- one match, success: placeholder with `path` and `name`
- one match, 401: fallback string includes `Original: <url>`
- one match, oversized: fallback
- multiple matches with one failure: successes use placeholders, failures
  use the fallback, the rest of the text is preserved
- reverse-span splicing does not corrupt other ranges

`uploader.Upload` (Client + `httptest` faking S3):

- success: returns `![name](assetUrl)` for images, `[name](assetUrl)` otherwise
- `FileUpload` mutation fails → error
- S3 PUT non-2xx → error
- missing file → error
- file exceeds `MaxMediaSize` → error before mutation

### `handler_test.go` (delta)

- `tickOnce` calls the resolver and dispatches messages whose `Content`
  contains the expected placeholders
- `Sender.Send` with `MediaPath != ""` calls the uploader and appends the
  markdown fragment after `selfMarker + Content`
- `MediaPath != "" && Content == ""` still sends (media-only)
- `MediaPath == ""` keeps the legacy code path (regression guard)

### `client_test.go` (delta)

- `DownloadAsset`: `httptest` verifies `Authorization` is forwarded, status
  codes are honored, `Content-Type` is returned
- `FileUpload`: GraphQL mock verifies variables and decodes the response
  shape

### Configuration regression

`config_bee_test.go` is extended to cover the `MaxMediaSize` default
(50 MB) and yaml override.

## Rollout / Compatibility

- New `MaxMediaSize` field defaults to 50 MB; existing yaml configs do not
  need updating.
- The SeenSet format and dedup rules are unchanged. Attachment resolution
  happens before `dispatch`, so it is orthogonal to the seen-tracking layer.
- Bot-self detection still uses `botCommentPrefix`; `selfMarker` continues
  to prefix the body, with `Content` followed by media markdown — old
  comments produced before this change keep matching the prefix.
- Outbound replies that do not set `MediaPath` follow the unchanged code
  path. The change is additive, so deploying carries no risk of regressing
  the text-only flow.
