# Multimedia Message Handling Design Spec

> Inbound media reception + basic outbound media sending for Feishu and DingTalk platforms.

---

## 1. Overview

RoboBee currently handles text-only messages. This design adds support for receiving and sending multimedia messages (images, files, audio, video) on both Feishu and DingTalk platforms.

### Scope

- **Inbound**: Parse, download, and process media messages from users. Pass media to AI via local file paths and text placeholders.
- **Outbound**: Allow workers to upload and send media files back to users via the existing `send_message` MCP tool.
- **Text extraction**: Extract inline text from PDF, DOCX, and plain text files.

### Out of scope (future work)

- DingTalk AI Card streaming
- DingTalk media markers (`[DINGTALK_VIDEO]`, etc.)
- FFmpeg-based video thumbnail generation
- Feishu interactive card sending

---

## 2. Data Structures

### 2.1 MediaFile (new)

```go
// platform/interfaces.go
type MediaFile struct {
    Path          string // local file path, e.g. ~/.robobee/media/inbound/1710000000-uuid.png
    ContentType   string // MIME type, e.g. "image/png"
    FileName      string // original file name (if available)
    Placeholder   string // e.g. "<media:image>", "<media:document>"
    ExtractedText string // for PDF/DOCX/text files — inline content (truncated to 50K chars)
}
```

### 2.2 InboundMessage (modified)

Add field:

```go
MediaFiles []MediaFile // downloaded media, empty for text-only messages
```

### 2.3 OutboundMessage (modified)

Add field:

```go
MediaPath string // optional local file path to upload and send
```

---

## 3. Package Structure

### New files

```
internal/
├── media/
│   ├── service.go          # MediaService: save to disk, MIME detection, placeholder mapping
│   ├── extract.go          # Text extraction: PDF, DOCX, plain text
│   └── extract_test.go
├── platform/
│   ├── feishu/
│   │   ├── post.go         # Post (rich text) parser
│   │   └── post_test.go
│   └── dingtalk/
│       └── token.go        # getAccessToken + getOAPIToken (both cached)
```

### Modified files

| File | Changes |
|---|---|
| `internal/platform/interfaces.go` | Add `MediaFile`, update `InboundMessage`, `OutboundMessage` |
| `internal/platform/feishu/handler.go` | Handle all msg_types, media download/upload/send |
| `internal/platform/dingtalk/handler.go` | Handle all msgtypes, downloadCode exchange, upload/send |
| `internal/mcp/tools.go` | Add `media_path` parameter to `send_message` |
| `go.mod` | Add PDF and DOCX libraries |

### New dependencies

| Purpose | Package |
|---|---|
| PDF text extraction | `github.com/ledongthuc/pdf` |
| DOCX text extraction | `github.com/nguyenthenguyen/docx` |
| MIME detection | `net/http.DetectContentType` (stdlib) |

---

## 4. Media Service (`internal/media/`)

### 4.1 service.go

```go
type Service struct {
    baseDir string // ~/.robobee/media
}

func NewService() *Service // baseDir = ~/.robobee/media, creates inbound/ and outbound/ subdirs

func (s *Service) SaveInbound(data []byte, ext string) (path string, err error)
// Saves to ~/.robobee/media/inbound/<timestamp>-<uuid>.<ext>

func (s *Service) DetectMIME(data []byte, fileName string) string
// Uses net/http.DetectContentType, falls back to extension-based mapping

func (s *Service) MediaPlaceholder(contentType string) string
// Maps MIME to placeholder:
//   image/*        → "<media:image>"
//   audio/*        → "<media:audio>"
//   video/*        → "<media:video>"
//   application/*  → "<media:document>"
//   default        → "<media:document>"

func (s *Service) ExtensionFromMIME(contentType string) string
// Maps MIME to file extension for saving
```

### 4.2 extract.go

```go
func (s *Service) ExtractText(path string) (string, error)
// Dispatches by file extension:
//   .txt .md .csv .json .xml .yaml .yml .html .log .conf .ini
//   .sh .py .js .ts .css .sql .go .java .rs .rb .php → read UTF-8, truncate 50K chars
//   .pdf → PDF library extraction
//   .docx → DOCX library extraction
//   other → return "", nil (no extraction, file still saved)
```

---

## 5. Inbound Media Flow

### 5.1 Message type handling

Both platform handlers remove their text-only filters and handle all message types.

### 5.2 Content field construction

| Platform | Message type | Content value |
|---|---|---|
| Feishu | text | Text content (unchanged) |
| Feishu | image | `<media:image>` |
| Feishu | file | `<media:document>` or extracted text |
| Feishu | audio | `<media:audio>` |
| Feishu | video/media | `<media:video>` |
| Feishu | sticker | `<media:sticker>` |
| Feishu | post | Rendered markdown + embedded media placeholders |
| DingTalk | text | Text content (unchanged) |
| DingTalk | picture | `<media:image>` |
| DingTalk | richText | Concatenated text + image placeholders |
| DingTalk | file | `<media:document>` or extracted text |
| DingTalk | audio | Recognition text if available, else `<media:audio>` |
| DingTalk | video | `<media:video>` |

### 5.3 Feishu download flow

For `image/file/audio/video/media/sticker` messages:

1. Parse content JSON → extract `image_key` / `file_key` / `file_name`
2. Determine resource type: `"image"` for image msg_type, `"file"` for all others
3. Call `client.Im.MessageResource.Get()` with `message_id`, `file_key`, `type`
4. Read response body → `[]byte`
5. Detect MIME → determine extension
6. Save via `media.Service.SaveInbound()`
7. Extract text if applicable
8. Build `MediaFile` struct

For `post` messages:

1. Parse post content (try 3 formats in order)
2. Render elements to markdown text
3. Collect embedded `image_key` and `file_key` lists
4. Download each via `messageResource.get`
5. Build `MediaFile` list

### 5.4 DingTalk download flow

For `picture` messages:

1. Extract `downloadCode` from `data.content`
2. Exchange for URL: `POST api.dingtalk.com/v1.0/robot/messageFiles/download` with `{downloadCode, robotCode}`
3. HTTP GET the download URL
4. Detect MIME from `Content-Type` header
5. Save and build `MediaFile`

For `richText` messages:

1. Iterate `data.content.richText[].part`
2. Collect `pictureUrl` values → HTTP GET download each
3. Collect `text` values → concatenate

For `file` messages:

1. Extract `downloadCode` + `fileName`
2. Exchange downloadCode for URL → download
3. Extract text if applicable (PDF/DOCX/plain text)

For `audio` messages:

1. Use `data.content.recognition` text if available
2. Otherwise: placeholder `<media:audio>`

For `video` messages:

1. Placeholder `<media:video>` (no download needed for AI)

### 5.5 Feishu post parser (`feishu/post.go`)

```go
type PostParseResult struct {
    TextContent string
    ImageKeys   []string
    MediaKeys   []struct{ FileKey, FileName string }
}

func ParsePostContent(content string) (*PostParseResult, error)
```

Tries three content shapes in order:
1. `{"title": "...", "content": [[...]]}` — direct
2. `{"zh_cn": {"title": "...", "content": [[...]]}}` — locale-wrapped
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

## 6. Outbound Media Flow

### 6.1 send_message tool changes

Add optional `media_path` parameter:

```go
var params struct {
    MessageID string `json:"message_id"`
    Content   string `json:"content"`
    MediaPath string `json:"media_path"` // optional
}
```

- `content` is now optional when `media_path` is set (at least one must be provided)
- If both set: send text first, then media as separate message
- `media_path` is passed through `OutboundMessage.MediaPath`

### 6.2 File type detection by extension

```go
// Image extensions
.jpg .jpeg .png .gif .webp .bmp .ico .tiff → image

// Audio extensions
.opus .ogg .mp3 .wav .amr .aac .flac .m4a → audio

// Video extensions
.mp4 .mov .avi → video

// Everything else → file
```

### 6.3 Feishu upload + send

**Image**:
- Upload: `POST /im/v1/images` multipart (`image_type: "message"`, `image: <data>`) → `image_key`
- Send: `msg_type: "image"`, content: `{"image_key": "..."}`

**File/Audio/Video**:
- Detect `file_type` from extension: opus, mp4, pdf, doc, xls, ppt, stream
- Upload: `POST /im/v1/files` multipart (`file_type`, `file_name`, `file: <data>`) → `file_key`
- Send `msg_type`: opus → `"audio"`, mp4 → `"media"`, others → `"file"`
- Content: `{"file_key": "..."}`

**Reply fallback**: If reply fails with error 230011/231003 (message withdrawn), fall back to `message.Create`.

**File name sanitization**: `[\x00-\x1F\x7F\r\n"\\]` → `_` (CWE-93 prevention). No percent-encoding.

### 6.4 DingTalk upload + send

**New token**: `getOAPIToken` — `GET oapi.dingtalk.com/gettoken?appkey=&appsecret=`, cached with same pattern as `getAccessToken`.

**Upload**: `POST oapi.dingtalk.com/media/upload?access_token=<oapiToken>&type=<mediaType>` multipart.
- `mediaType`: `image` | `file` | `video` | `voice`
- Content-Type in multipart: `image/jpeg` for images, `application/octet-stream` for others

**Send via sessionWebhook**:

| File type | msgtype | Payload |
|---|---|---|
| image | `image` | Not used for passive reply (use markdown with media_id) |
| file | `file` | `{"mediaId": "...", "fileName": "...", "fileType": "..."}` |
| audio | `voice` | `{"mediaId": "...", "duration": "60000"}` |
| video | `video` | `{"duration": "0", "videoMediaId": "...", "videoType": "mp4", "picMediaId": ""}` |

---

## 7. Error Handling

### 7.1 Inbound errors

- **Download failure**: Log error, continue processing. AI sees placeholder but no file path. Non-blocking.
- **Text extraction failure**: Fall back to `<media:document>` placeholder. File still saved locally.
- **Timeout**: 120s for all media downloads.

### 7.2 Outbound errors

- **Upload failure**: Return error to worker via MCP tool response. No automatic retry.
- **File too large**: Check before upload. Feishu: 30MB limit. DingTalk: 20MB limit. Return descriptive error.
- **File not found**: Return error to worker.

### 7.3 Security

- **Inbound file naming**: `<timestamp>-<uuid>.<ext>` — never uses user-provided filenames for local paths.
- **Feishu key validation**: `image_key` and `file_key` validated against safe pattern before API calls.
- **Upload file name sanitization**: Control characters stripped (CWE-93).

---

## 8. Unchanged Components

- **Message ingestion gateway** (`msgingest/`): `InboundMessage` flows through with additional `MediaFiles` data.
- **Bee/worker pipeline**: Workers see `Content` field with media placeholders + `MediaFiles` paths.
- **Database schema**: No changes. `content` stores text with placeholders, `raw` stores original event.
- **Configuration**: No new config fields. `~/.robobee/media/` path is conventional.

---

## 9. Storage Convention

```
~/.robobee/
├── media/
│   ├── inbound/      # Downloaded media from users
│   │   └── 1710000000-550e8400-e29b.png
│   └── outbound/     # Reserved for future use
├── bee/              # Existing
└── worker/           # Existing
```

File naming: `<unix_timestamp>-<uuid>.<extension>`
