# Multimedia Message Handling Design Spec

> Inbound media reception + basic outbound media sending for Feishu and DingTalk platforms.

---

## 1. Overview

RoboBee currently handles text-only messages. This design adds support for receiving and sending multimedia messages (images, files, audio, video) on both Feishu and DingTalk platforms.

### Scope

- **Inbound**: Parse, download, and process media messages from users. Pass media info to AI via the `Content` field (file paths + placeholders embedded in text).
- **Outbound**: Allow workers to upload and send media files back to users via the existing `send_message` MCP tool.
- **Text extraction**: Extract inline text from PDF, DOCX, and plain text files.

### Out of scope (future work)

- DingTalk AI Card streaming
- DingTalk media markers (`[DINGTALK_VIDEO]`, etc.)
- FFmpeg-based video thumbnail generation
- Feishu interactive card sending
- Media file cleanup/retention policy (files accumulate in `~/.robobee/media/inbound/`)

---

## 2. Key Design Decision: Content-Embedded Media Paths

The existing message pipeline stores messages to SQLite (`platform_messages` table) and the Feeder reads back only `id, session_key, platform, content` via `ClaimedMessage`. Adding a `MediaFiles` field to `InboundMessage` would be lost during this database round-trip.

**Solution**: Embed media file paths directly into the `Content` field. No database schema changes needed, and media info naturally flows through the entire pipeline to the AI.

Format examples:
- Image: `<media:image path="/Users/x/.robobee/media/inbound/1710000000-uuid.png">`
- File with extracted text: `<media:document name="report.pdf" path="/Users/x/.robobee/media/inbound/1710000000-uuid.pdf">\n` + extracted text content
- Audio with recognition: `<media:audio>` + recognition text
- Video: `<media:video path="/Users/x/.robobee/media/inbound/1710000000-uuid.mp4">`
- Sticker: `<media:sticker path="/Users/x/.robobee/media/inbound/1710000000-uuid.png">` (special-cased in handler, not derived from MIME)

This means `InboundMessage` does **not** get a `MediaFiles` field. The `Content` field carries everything.

---

## 3. Data Structures

### 3.1 OutboundMessage (modified)

Add field:

```go
MediaPath string // optional local file path to upload and send
```

### 3.2 InboundMessage — unchanged

No struct changes. Media info is embedded in the existing `Content` string field.

---

## 4. Package Structure

### New files

```
internal/
├── media/
│   ├── service.go          # MediaService: save to disk, MIME detection, placeholder building
│   ├── extract.go          # Text extraction: PDF, DOCX, plain text
│   └── extract_test.go
├── platform/
│   ├── feishu/
│   │   ├── post.go         # Post (rich text) parser
│   │   └── post_test.go
│   └── dingtalk/
│       └── token.go        # Move getAccessToken here + add getOAPIToken (both cached)
```

### Modified files

| File | Changes |
|---|---|
| `internal/platform/interfaces.go` | Add `MediaPath` to `OutboundMessage` |
| `internal/platform/feishu/handler.go` | Handle all msg_types, media download in receiver, upload+send in sender. Use Lark SDK for all API calls. |
| `internal/platform/dingtalk/handler.go` | Handle all msgtypes, downloadCode exchange, upload/send. Move token code to `token.go`. |
| `internal/mcp/tools.go` | Add `media_path` parameter to `send_message` tool schema and handler |
| `go.mod` | Add PDF and DOCX libraries |

### New dependencies

| Purpose | Package |
|---|---|
| PDF text extraction | `github.com/ledongthuc/pdf` |
| DOCX text extraction | `github.com/nguyenthenguyen/docx` |
| MIME detection | `net/http.DetectContentType` (stdlib) |

---

## 5. Media Service (`internal/media/`)

### 5.1 service.go

```go
type Service struct {
    baseDir string // ~/.robobee/media
}

func NewService() *Service // baseDir = ~/.robobee/media, creates inbound/ subdir

func (s *Service) SaveInbound(ctx context.Context, data []byte, ext string) (path string, err error)
// Saves to ~/.robobee/media/inbound/<timestamp>-<uuid>.<ext>

func (s *Service) DetectMIME(data []byte, fileName string) string
// Uses net/http.DetectContentType, falls back to extension-based mapping

func (s *Service) ExtensionFromMIME(contentType string) string
// Maps MIME to file extension for saving

func (s *Service) BuildPlaceholder(mediaType string, path string, fileName string) string
// Builds content placeholder string, e.g.:
//   BuildPlaceholder("image", "/path/to/file.png", "") → `<media:image path="/path/to/file.png">`
//   BuildPlaceholder("document", "/path/to/f.pdf", "report.pdf") → `<media:document name="report.pdf" path="/path/to/f.pdf">`
// mediaType is one of: "image", "audio", "video", "document", "sticker"

func MediaTypeFromMIME(contentType string) string
// Maps MIME prefix to media type string:
//   image/*        → "image"
//   audio/*        → "audio"
//   video/*        → "video"
//   default        → "document"
```

### 5.2 extract.go

```go
func (s *Service) ExtractText(ctx context.Context, path string) (string, error)
// Dispatches by file extension:
//   .txt .md .csv .json .xml .yaml .yml .html .log .conf .ini
//   .sh .py .js .ts .css .sql .go .java .rs .rb .php → read UTF-8, truncate 50K chars
//   .pdf → PDF library extraction
//   .docx → DOCX library extraction
//   other → return "", nil (no extraction, file still saved)
```

---

## 6. Inbound Media Flow

### 6.1 Message type handling

Both platform handlers remove their text-only filters and handle all message types. Unrecognized message types (`merge_forward`, `interactive`, and any future types) are logged at WARN level and silently skipped (return nil, no dispatch).

### 6.2 Content field construction

| Platform | Message type | Content value |
|---|---|---|
| Feishu | text | Text content (unchanged) |
| Feishu | image | `<media:image path="...">` |
| Feishu | file | `<media:document name="..." path="...">` + extracted text (if applicable) |
| Feishu | audio | `<media:audio path="...">` |
| Feishu | video/media | `<media:video path="...">` |
| Feishu | sticker | `<media:sticker path="...">` (special-cased, not from MIME) |
| Feishu | post | Rendered markdown + embedded media placeholders with paths |
| Feishu | merge_forward | Log and skip |
| Feishu | interactive | Log and skip |
| DingTalk | text | Text content (unchanged) |
| DingTalk | picture | `<media:image path="...">` |
| DingTalk | richText | Concatenated text + `<media:image path="...">` for each picture |
| DingTalk | file | `<media:document name="..." path="...">` + extracted text (if applicable) |
| DingTalk | audio | Recognition text if available, else `<media:audio>` (no download) |
| DingTalk | video | `<media:video>` (placeholder only, no download) |

### 6.3 Feishu download flow

All Feishu media API calls use the **Lark Go SDK** (`github.com/larksuite/oapi-sdk-go/v3`), consistent with the existing codebase. No raw HTTP calls for Feishu.

For `image/file/audio/video/media/sticker` messages:

1. Parse content JSON → extract `image_key` / `file_key` / `file_name`
2. Determine resource type: `"image"` for image msg_type, `"file"` for all others
3. Call Lark SDK `client.Im.MessageResource.Get()` with `message_id`, `file_key`, `type`
4. Read response body → `[]byte`
5. Detect MIME → determine extension
6. Save via `media.Service.SaveInbound()`
7. Extract text if applicable
8. Build content string with `media.Service.BuildPlaceholder()`

For `post` messages:

1. Parse post content (try 3 formats in order)
2. Render elements to markdown text
3. Collect embedded `image_key` and `file_key` lists
4. Download each via Lark SDK `client.Im.MessageResource.Get()`
5. Append media placeholders with paths to rendered text

Timeout: 120s for all media download operations.

### 6.4 DingTalk download flow

DingTalk uses raw `net/http` calls (no Go SDK for media operations), consistent with the existing approach.

For `picture` messages:

1. Extract `downloadCode` from `data.content`
2. Exchange for URL: `POST api.dingtalk.com/v1.0/robot/messageFiles/download` with `{downloadCode, robotCode}`
3. HTTP GET the download URL
4. Detect MIME from `Content-Type` header
5. Save and build content with placeholder

For `richText` messages:

1. Iterate `data.content.richText[].part`
2. Collect `pictureUrl` values → HTTP GET download each
3. Collect `text` values → concatenate
4. Build content: text parts + image placeholders with paths

For `file` messages:

1. Extract `downloadCode` + `fileName`
2. Exchange downloadCode for URL → download
3. Extract text if applicable (PDF/DOCX/plain text)

For `audio` messages:

1. Use `data.content.recognition` text if available
2. Otherwise: placeholder `<media:audio>` (no download)

For `video` messages:

1. Placeholder `<media:video>` (no download needed for AI)

Timeout: 120s for all media download operations.

### 6.5 Feishu post parser (`feishu/post.go`)

```go
type PostParseResult struct {
    TextContent string
    ImageKeys   []string
    MediaKeys   []MediaKeyInfo
}

type MediaKeyInfo struct {
    FileKey  string
    FileName string
}

func ParsePostContent(content string) (*PostParseResult, error)
```

Note: `at` elements' `open_id` values are intentionally not collected. The post parser focuses on text rendering and media key extraction. User mentions are rendered as `@user_name` in the text content.

Tries three content shapes in order:
1. `{"title": "...", "content": [[...]]}` — direct
2. `{"zh_cn": {"title": "...", "content": [[...]]}}` — locale-wrapped (tries all locale keys)
3. `{"post": {"zh_cn": {...}}}` — double-wrapped

Element rendering:

| Tag | Rendering |
|---|---|
| `text` | Plain text with style (bold → `**text**`, italic → `*text*`, code → `` `text` ``, strikethrough → `~~text~~`) |
| `a` | `[text](href)` |
| `at` | `@user_name` |
| `img` | Extract `image_key`, add to download list |
| `media` | Extract `file_key` + `file_name`, add to download list |
| `code_block` / `pre` | ` ```lang\ncode\n``` ` |
| `code` | `` `text` `` |
| `emotion` | Emoji text |
| `br` | `\n` |
| `hr` | `---` |

---

## 7. Outbound Media Flow

### 7.1 send_message tool changes

Add optional `media_path` parameter. Update the JSON Schema: remove `content` from `required`, add validation that at least one of `content` or `media_path` must be non-empty.

```go
var params struct {
    MessageID string `json:"message_id"`
    Content   string `json:"content"`    // optional if media_path is set
    MediaPath string `json:"media_path"` // optional, local file path
}

// Validation replaces the existing `if params.Content == ""` check:
if params.Content == "" && params.MediaPath == "" {
    return nil, fmt.Errorf("at least one of 'content' or 'media_path' must be provided")
}
```

- If both set: send text first, then media as a separate message
- `media_path` is passed through `OutboundMessage.MediaPath`
- `media_path` points to files the worker produced in its own `work_dir` — no path restriction needed since workers run locally

### 7.2 File type detection by extension

```go
// Image extensions
.jpg .jpeg .png .gif .webp .bmp .ico .tiff → image

// Audio extensions
.opus .ogg .mp3 .wav .amr .aac .flac .m4a → audio

// Video extensions
.mp4 .mov .avi → video

// Everything else → file
```

### 7.3 Feishu upload + send

All operations use the **Lark Go SDK**, consistent with existing code.

**Image**:
- Upload: Lark SDK `client.Im.Image.Create()` with `image_type: "message"` → `image_key`
- Send: `msg_type: "image"`, content: `{"image_key": "..."}`

**File/Audio/Video**:
- Detect `file_type` from extension: opus, mp4, pdf, doc, xls, ppt, stream
- Upload: Lark SDK `client.Im.File.Create()` with `file_type`, `file_name` → `file_key`
- Send `msg_type`: opus → `"audio"`, mp4 → `"media"`, others → `"file"`
- Content: `{"file_key": "..."}`

**Reply fallback**: If reply fails with error 230011/231003 (message withdrawn), fall back to `message.Create`.

**File name sanitization**: `[\x00-\x1F\x7F\r\n"\\]` → `_` (CWE-93 prevention). No percent-encoding.

Upload timeout: 120s. Send timeout: 30s (default).

### 7.4 DingTalk upload + send

**Token management**: Move existing `getAccessToken` from `handler.go` to new `token.go` file. Add `getOAPIToken` alongside it with its own cache variables (`oapiTokenCache`, `oapiTokenMu`).

```go
// token.go

// getAccessToken — existing, moved here. New API token.
// POST https://api.dingtalk.com/v1.0/oauth2/accessToken

// getOAPIToken — new. Legacy OAPI token for media upload.
// GET https://oapi.dingtalk.com/gettoken?appkey=&appsecret=
// Same caching pattern: cache token + expiresAt, refresh 60s before expiry.
```

**Upload**: `POST oapi.dingtalk.com/media/upload?access_token=<oapiToken>&type=<mediaType>` multipart.
- `mediaType`: `image` | `file` | `video` | `voice`
- Content-Type in multipart: `image/jpeg` for images, `application/octet-stream` for others
- Upload timeout: 60s

**Send via sessionWebhook**:

| File type | msgtype | Payload |
|---|---|---|
| image | `markdown` | `{"title": "Image", "text": "![image](media_id)"}` — DingTalk passive reply sends images embedded in markdown |
| file | `file` | `{"mediaId": "...", "fileName": "...", "fileType": "..."}` |
| audio | `voice` | `{"mediaId": "...", "duration": "60000"}` |
| video | `video` | `{"duration": "0", "videoMediaId": "...", "videoType": "mp4", "picMediaId": ""}` |

---

## 8. Error Handling

### 8.1 Inbound errors

- **Download failure**: Log error, continue processing. Content gets placeholder without path (e.g. `<media:image>` with no `path` attribute). Non-blocking.
- **Text extraction failure**: Fall back to placeholder with path only. File still saved locally.
- **Timeout**: 120s for all media downloads.

### 8.2 Outbound errors

- **Upload failure**: Return error to worker via MCP tool response. No automatic retry.
- **Upload timeout**: 120s for Feishu, 60s for DingTalk.
- **File too large**: Check before upload. Feishu: 30MB limit. DingTalk: 20MB limit. Return descriptive error.
- **File not found**: Return error to worker.

### 8.3 Security

- **Inbound file naming**: `<timestamp>-<uuid>.<ext>` — never uses user-provided filenames for local paths.
- **Feishu key validation**: `image_key` and `file_key` validated against alphanumeric+underscore pattern before API calls, preventing path traversal.
- **Upload file name sanitization**: Control characters `[\x00-\x1F\x7F\r\n"\\]` stripped → `_` (CWE-93).

---

## 9. Unchanged Components

- **Message ingestion gateway** (`msgingest/`): `InboundMessage` flows through unchanged. Media info is in the `Content` field.
- **Bee/worker pipeline**: Workers see `Content` field with embedded media placeholders and file paths.
- **Database schema**: No changes. `content` stores text with media placeholders/paths, `raw` stores original event.
- **Configuration**: No new config fields. `~/.robobee/media/` path is conventional.

---

## 10. Storage Convention

```
~/.robobee/
├── media/
│   └── inbound/      # Downloaded media from users
│       └── 1710000000-550e8400-e29b.png
├── bee/              # Existing
└── worker/           # Existing
```

File naming: `<unix_timestamp>-<uuid>.<extension>`
