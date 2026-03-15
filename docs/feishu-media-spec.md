# 飞书（Lark）媒体消息处理技术实现报告

> 本文档总结了当前 TypeScript 项目中飞书扩展对图片、视频、音频、文档等多媒体消息的接收与发送的底层技术实现，供 Golang 重新实现参考。

---

## 1. 总体架构

```
┌─────────────────────────────────────────────────────┐
│                   飞书开放平台                        │
│  (WebSocket / Webhook 事件推送 + REST API)           │
└──────────┬──────────────────────────────┬────────────┘
           │ 事件接收                      │ API 调用
           ▼                              ▼
┌─────────────────┐              ┌─────────────────────┐
│  事件监听层       │              │  消息发送层           │
│  (monitor)       │              │  (send / media)      │
│  - WebSocket     │              │  - 文本/卡片发送      │
│  - Webhook       │              │  - 媒体上传+发送      │
└────────┬────────┘              └──────────┬──────────┘
         │                                  │
         ▼                                  ▼
┌─────────────────┐              ┌─────────────────────┐
│  消息解析层       │              │  媒体处理层           │
│  (bot / post)    │              │  (media)             │
│  - 类型识别       │              │  - 上传图片/文件      │
│  - 内容解析       │              │  - 下载图片/资源      │
│  - 媒体key提取   │              │  - MIME类型检测       │
└────────┬────────┘              └───────────────────────┘
         │
         ▼
┌─────────────────┐
│  媒体下载+存储    │
│  - 下载到Buffer   │
│  - MIME检测       │
│  - 保存到本地磁盘  │
└─────────────────┘
```

---

## 2. 飞书 API 端点汇总

| 操作 | API 端点 | SDK 方法 | 说明 |
|------|---------|---------|------|
| 下载图片 | `GET /im/v1/images/{image_key}` | `client.im.image.get()` | 通过 image_key 下载图片 |
| 下载消息资源 | `GET /im/v1/messages/{message_id}/resources/{file_key}` | `client.im.messageResource.get()` | 下载文件/音频/视频/嵌入图片，需指定 `type=image\|file` |
| 上传图片 | `POST /im/v1/images` | `client.im.image.create()` | multipart 上传，返回 `image_key` |
| 上传文件 | `POST /im/v1/files` | `client.im.file.create()` | multipart 上传，返回 `file_key` |
| 发送消息 | `POST /im/v1/messages` | `client.im.message.create()` | 发送新消息 |
| 回复消息 | `POST /im/v1/messages/{message_id}/reply` | `client.im.message.reply()` | 回复已有消息 |
| 更新卡片 | `PATCH /im/v1/messages/{message_id}` | `client.im.message.patch()` | 更新交互卡片内容 |

---

## 3. 接收消息 —— 媒体类型识别与解析

### 3.1 支持的消息类型

飞书推送的消息事件中，`msg_type` 字段标识消息类型：

| msg_type | 说明 | 媒体 key 字段 | 下载方式 |
|----------|------|-------------|---------|
| `text` | 纯文本 | 无 | 无需下载 |
| `post` | 富文本（可嵌入图片/视频） | 内嵌 `image_key` / `file_key` | `messageResource.get` |
| `image` | 图片消息 | `image_key` | `messageResource.get(type=image)` |
| `file` | 文件/文档消息 | `file_key` + `file_name` | `messageResource.get(type=file)` |
| `audio` | 语音消息 | `file_key` | `messageResource.get(type=file)` |
| `video` / `media` | 视频消息 | `file_key`(视频) + `image_key`(缩略图) | `messageResource.get(type=file)` |
| `sticker` | 表情贴纸 | `file_key` | `messageResource.get(type=file)` |
| `merge_forward` | 合并转发 | 递归解析子消息 | 逐条处理 |
| `interactive` | 交互卡片 | 无 | 无需下载 |

### 3.2 消息内容解析 (parseMediaKeys)

消息的 `content` 字段为 JSON 字符串，根据 `msg_type` 解析不同的 key：

```go
// 伪代码
func parseMediaKeys(content string, msgType string) (imageKey, fileKey, fileName string) {
    parsed := json.Unmarshal(content)
    switch msgType {
    case "image":
        return parsed.image_key, "", ""
    case "file":
        return "", parsed.file_key, parsed.file_name
    case "audio":
        return "", parsed.file_key, ""
    case "video", "media":
        return parsed.image_key, parsed.file_key, ""  // 视频有缩略图+视频文件
    case "sticker":
        return "", parsed.file_key, ""
    }
}
```

### 3.3 资源类型映射 (toMessageResourceType)

调用 `messageResource.get` 时需要指定 `type` 参数：

```go
func toMessageResourceType(msgType string) string {
    if msgType == "image" {
        return "image"
    }
    return "file"  // 所有非图片类型统一用 "file"
}
```

**重要**：飞书 API 限制，`messageResource.get` 的 `type` 参数只支持 `"image"` 和 `"file"` 两种值。非图片类型的媒体一律用 `"file"`。

### 3.4 富文本 (post) 消息解析

富文本消息的 content 结构较复杂，支持多种嵌套格式：

#### 3.4.1 Post Payload 结构

```json
// 直接格式
{ "title": "标题", "content": [[elements...]] }

// 带语言locale的格式
{ "zh_cn": { "title": "标题", "content": [[elements...]] } }

// 包裹在 post 字段中
{ "post": { "zh_cn": { "title": "标题", "content": [[elements...]] } } }
```

解析时需按优先级依次尝试这三种格式。

#### 3.4.2 Post 元素类型

| tag | 说明 | 关键字段 | 渲染输出 |
|-----|------|---------|---------|
| `text` | 文本 | `text`, `style`(bold/italic/code/strikethrough) | Markdown 格式文本 |
| `a` | 链接 | `text`, `href` | `[text](href)` |
| `at` | @提及 | `user_id`, `open_id`, `user_name` | `@name` |
| `img` | 内嵌图片 | `image_key` | 提取 image_key 用于下载 |
| `media` | 内嵌媒体 | `file_key`, `file_name` | 提取 file_key 用于下载 |
| `code_block` / `pre` | 代码块 | `text`/`content`, `language` | ` ```lang\ncode\n``` ` |
| `code` | 行内代码 | `text` | `` `code` `` |
| `emotion` | 表情 | `emoji`, `emoji_type` | emoji 文本 |
| `br` | 换行 | 无 | `\n` |
| `hr` | 分割线 | 无 | `---` |

#### 3.4.3 Post 解析返回值

```go
type PostParseResult struct {
    TextContent      string                        // 渲染后的纯文本/Markdown
    ImageKeys        []string                      // 嵌入的图片 image_key 列表
    MediaKeys        []struct{ FileKey, FileName string } // 嵌入的媒体 file_key 列表
    MentionedOpenIds []string                      // 被 @ 的用户 open_id 列表
}
```

---

## 4. 接收消息 —— 媒体下载流程

### 4.1 下载流程 (resolveFeishuMediaList)

```
消息事件到达
    │
    ├─ msg_type ∈ {image, file, audio, video, media, sticker}
    │   │
    │   ├─ 1. parseMediaKeys() 提取 imageKey / fileKey
    │   ├─ 2. downloadMessageResourceFeishu() 下载到 Buffer
    │   ├─ 3. detectMime() 检测 MIME 类型（如果响应未提供）
    │   └─ 4. saveMediaBuffer() 保存到本地磁盘
    │
    └─ msg_type == "post"
        │
        ├─ 1. parsePostContent() 提取 imageKeys[] 和 mediaKeys[]
        ├─ 2. 遍历 imageKeys:
        │      downloadMessageResourceFeishu(type="image") → detectMime → save
        └─ 3. 遍历 mediaKeys:
               downloadMessageResourceFeishu(type="file") → detectMime → save
```

### 4.2 下载图片 API (downloadImageFeishu)

```
GET /im/v1/images/{image_key}

请求参数:
  - path.image_key: 图片的 image_key（需先做安全校验/规范化）

响应:
  - 二进制图片数据（可能是 Buffer / ArrayBuffer / Stream / 异步迭代器等多种格式）

超时: 120秒
```

### 4.3 下载消息资源 API (downloadMessageResourceFeishu)

```
GET /im/v1/messages/{message_id}/resources/{file_key}?type={image|file}

请求参数:
  - path.message_id: 消息 ID
  - path.file_key: 资源的 file_key（需安全校验/规范化）
  - params.type: "image" 或 "file"

响应:
  - 二进制文件数据

超时: 120秒
```

### 4.4 响应数据读取

飞书 SDK 返回的响应可能是多种格式，需要按以下优先级处理：

```go
// Golang 实现时简化为直接读取 HTTP Response Body
// TypeScript 版本需要处理以下格式（因 SDK 封装差异）:
// 1. Buffer 直接返回
// 2. ArrayBuffer → 转 Buffer
// 3. response.data 为 Buffer/ArrayBuffer
// 4. response.getReadableStream() → 流式读取
// 5. response.writeFile(tmpPath) → 写临时文件再读取
// 6. AsyncIterator → 逐块读取
// 7. Readable stream → 逐块读取
```

**Golang 实现建议**：直接使用 `http.Client` 调用飞书 REST API，读取 `resp.Body` 即可，无需处理上述复杂的 SDK 封装。

### 4.5 媒体占位符

下载的媒体在文本中用占位符表示：

| msg_type | 占位符 |
|----------|--------|
| image | `<media:image>` |
| file | `<media:document>` |
| audio | `<media:audio>` |
| video / media | `<media:video>` |
| sticker | `<media:sticker>` |

### 4.6 媒体信息结构

```go
type FeishuMediaInfo struct {
    Path        string // 本地文件路径
    ContentType string // MIME 类型，如 "image/png"
    Placeholder string // 占位符文本，如 "<media:image>"
}
```

---

## 5. 发送消息 —— 媒体上传与发送流程

### 5.1 统一媒体发送流程 (sendMediaFeishu)

```
输入: mediaUrl 或 mediaBuffer + fileName
    │
    ├─ 1. 加载媒体数据
    │      - 如果是 mediaBuffer: 直接使用
    │      - 如果是 mediaUrl (HTTP/HTTPS): 下载到 Buffer
    │      - 如果是本地路径: 需验证 localRoots 安全白名单
    │      - 最大文件大小: 30MB (可配置 mediaMaxMb)
    │
    ├─ 2. 判断文件类型 (根据扩展名)
    │      - 图片: .jpg .jpeg .png .gif .webp .bmp .ico .tiff
    │      - 其他: 走文件类型检测 detectFileType()
    │
    ├─ 3a. 如果是图片:
    │      ├─ uploadImageFeishu() → 获得 image_key
    │      └─ sendImageFeishu() → 发送图片消息
    │
    └─ 3b. 如果是文件/音视频:
           ├─ detectFileType() → 确定 fileType
           ├─ uploadFileFeishu() → 获得 file_key
           └─ sendFileFeishu() → 发送文件消息 (msg_type 由 fileType 决定)
```

### 5.2 上传图片 (uploadImageFeishu)

```
POST /im/v1/images
Content-Type: multipart/form-data

字段:
  - image_type: "message" (消息图片) 或 "avatar" (头像)
  - image: 二进制图片数据 (Buffer 或文件流)

支持格式: JPEG, PNG, WEBP, GIF, TIFF, BMP, ICO

响应:
  { "image_key": "img_v3_xxxx" }
```

### 5.3 上传文件 (uploadFileFeishu)

```
POST /im/v1/files
Content-Type: multipart/form-data

字段:
  - file_type: "opus" | "mp4" | "pdf" | "doc" | "xls" | "ppt" | "stream"
  - file_name: 文件名（需安全处理）
  - file: 二进制文件数据
  - duration: (可选) 音视频时长，单位毫秒

最大文件大小: 30MB

响应:
  { "file_key": "file_v3_xxxx" }
```

### 5.4 文件类型检测 (detectFileType)

根据文件扩展名映射到飞书支持的文件类型：

```go
func detectFileType(fileName string) string {
    ext := strings.ToLower(filepath.Ext(fileName))
    switch ext {
    case ".opus", ".ogg":
        return "opus"
    case ".mp4", ".mov", ".avi":
        return "mp4"
    case ".pdf":
        return "pdf"
    case ".doc", ".docx":
        return "doc"
    case ".xls", ".xlsx":
        return "xls"
    case ".ppt", ".pptx":
        return "ppt"
    default:
        return "stream"
    }
}
```

### 5.5 文件名安全处理 (sanitizeFileNameForUpload)

```go
// 移除控制字符和注入向量（CWE-93 防护），保留 UTF-8 字符（中文、emoji 等）
func sanitizeFileNameForUpload(fileName string) string {
    re := regexp.MustCompile(`[\x00-\x1F\x7F\r\n"\\]`)
    return re.ReplaceAllString(fileName, "_")
}
```

**注意**：不要对文件名做 percent-encoding，飞书 API 会原样显示文件名，编码后会变成乱码。

### 5.6 发送图片消息 (sendImageFeishu)

```
POST /im/v1/messages
或
POST /im/v1/messages/{message_id}/reply

参数:
  - receive_id_type: "chat_id" | "open_id" | "user_id" | "union_id" | "email"
  - receive_id: 接收方 ID
  - msg_type: "image"
  - content: '{"image_key": "img_v3_xxxx"}'
  - reply_in_thread: true/false (可选，话题回复)
```

### 5.7 发送文件/音频/视频消息 (sendFileFeishu)

```
POST /im/v1/messages
或
POST /im/v1/messages/{message_id}/reply

参数:
  - receive_id_type: 同上
  - receive_id: 接收方 ID
  - msg_type: 根据文件类型决定
      - opus → "audio" (语音消息，飞书内可直接播放)
      - mp4 → "media" (视频消息，飞书内可直接播放)
      - 其他 → "file" (文件消息，显示为文件附件)
  - content: '{"file_key": "file_v3_xxxx"}'
  - reply_in_thread: true/false (可选)
```

### 5.8 msg_type 映射规则

```go
func resolveMediaMsgType(fileType string) string {
    switch fileType {
    case "opus":
        return "audio"   // 语音消息
    case "mp4":
        return "media"   // 可播放视频
    default:
        return "file"    // 文件附件
    }
}
```

---

## 6. 发送文本消息

### 6.1 Post 格式发送 (富文本 Markdown)

飞书发送文本消息使用 `post` 类型以支持 Markdown 渲染：

```json
{
  "msg_type": "post",
  "content": "{\"zh_cn\":{\"content\":[[{\"tag\":\"md\",\"text\":\"你好 **世界**\"}]]}}"
}
```

### 6.2 交互卡片发送

用于富格式展示（代码块、表格等），使用 schema 2.0：

```json
{
  "msg_type": "interactive",
  "content": "{\"schema\":\"2.0\",\"config\":{\"wide_screen_mode\":true},\"body\":{\"elements\":[{\"tag\":\"markdown\",\"content\":\"# 标题\\n内容\"}]}}"
}
```

---

## 7. 回复策略与容错

### 7.1 回复降级机制

```
尝试回复原消息 (client.im.message.reply)
    │
    ├─ 成功 → 返回
    │
    └─ 失败（消息已撤回/不存在）
        │
        ├─ 错误码 230011 或 231003
        ├─ 或错误信息包含 "withdrawn" / "not found"
        │
        └─ 降级为直接发送 (client.im.message.create)
```

### 7.2 话题回复

当 `reply_in_thread: true` 时，回复会创建/加入飞书话题（Topic Thread），而非行内回复。

---

## 8. 认证与客户端管理

### 8.1 客户端配置

```go
type FeishuClientConfig struct {
    AppID         string // 应用 App ID
    AppSecret     string // 应用 App Secret
    Domain        string // "feishu" | "lark" | 自定义域名（私有化部署）
    HttpTimeoutMs int    // HTTP 超时（默认 30秒，媒体操作 120秒，最大 300秒）
    EncryptKey    string // Webhook 加密 key
    VerifyToken   string // Webhook 验证 token
}
```

### 8.2 域名解析

```go
func resolveDomain(domain string) string {
    switch domain {
    case "feishu", "":
        return "https://open.feishu.cn"  // 飞书（国内版）
    case "lark":
        return "https://open.larksuite.com"  // Lark（国际版）
    default:
        return domain  // 私有化部署自定义域名
    }
}
```

---

## 9. 事件接收传输层

### 9.1 WebSocket 模式

- 使用飞书 SDK 的 WebSocket 客户端
- 长连接，自动重连
- 适合开发/内网环境

### 9.2 Webhook 模式

- HTTP 服务器监听事件推送
- 默认端口 3000，路径 `/feishu/events`
- 签名验证：HMAC-SHA256
- 请求体大小限制和超时可配置

---

## 10. 安全要点

### 10.1 External Key 规范化

所有 `image_key` 和 `file_key` 在使用前必须经过 `normalizeFeishuExternalKey()` 安全校验，防止路径遍历和注入攻击。

### 10.2 文件名安全

上传文件时，文件名需过滤控制字符（`\x00-\x1F`, `\x7F`, `\r`, `\n`, `"`, `\`），防止 multipart 注入（CWE-93）。

### 10.3 本地路径访问控制

从本地路径加载媒体时，必须验证路径在允许的 `localRoots` 白名单中（CVE-2026-26321 防护）。

---

## 11. 超时配置

| 操作类型 | 默认超时 | 说明 |
|---------|---------|------|
| 普通 API 调用 | 30 秒 | 文本消息发送、用户信息查询等 |
| 媒体操作 | 120 秒 | 图片/文件上传和下载 |
| 最大允许超时 | 300 秒 | 配置项上限 |

---

## 12. Golang 实现建议

### 12.1 HTTP 客户端

不建议使用飞书 Go SDK 的 `lark` 包封装（如有），建议直接使用 `net/http` 调用 REST API，这样：
- 响应处理更简单（直接读 `resp.Body`）
- 更好的超时控制
- 更灵活的 multipart 构建

### 12.2 关键数据结构

```go
// 媒体下载结果
type DownloadResult struct {
    Buffer      []byte
    ContentType string
    FileName    string
}

// 媒体上传结果
type UploadImageResult struct {
    ImageKey string `json:"image_key"`
}

type UploadFileResult struct {
    FileKey string `json:"file_key"`
}

// 发送结果
type SendResult struct {
    MessageID string
    ChatID    string
}

// 媒体信息（下载后的本地文件）
type MediaInfo struct {
    Path        string // 本地文件路径
    ContentType string // MIME 类型
    Placeholder string // 占位符文本
}

// 富文本解析结果
type PostParseResult struct {
    TextContent      string
    ImageKeys        []string
    MediaKeys        []MediaKeyInfo
    MentionedOpenIds []string
}

type MediaKeyInfo struct {
    FileKey  string
    FileName string
}
```

### 12.3 推荐的包

| 功能 | 推荐的 Go 包 |
|------|-------------|
| HTTP 客户端 | `net/http` |
| JSON 处理 | `encoding/json` |
| Multipart 上传 | `mime/multipart` |
| MIME 类型检测 | `net/http.DetectContentType` 或 `github.com/gabriel-vasile/mimetype` |
| WebSocket | `github.com/gorilla/websocket` |
| HMAC-SHA256 签名 | `crypto/hmac` + `crypto/sha256` |
| 文件路径处理 | `path/filepath` |
| 正则表达式 | `regexp` |

### 12.4 核心接口设计建议

```go
type FeishuMediaService interface {
    // 下载
    DownloadImage(imageKey string) (*DownloadResult, error)
    DownloadMessageResource(messageID, fileKey string, resourceType string) (*DownloadResult, error)

    // 上传
    UploadImage(data []byte, imageType string) (imageKey string, err error)
    UploadFile(data []byte, fileName, fileType string, duration *int) (fileKey string, err error)

    // 发送
    SendImage(to, imageKey string, opts *SendOptions) (*SendResult, error)
    SendFile(to, fileKey string, msgType string, opts *SendOptions) (*SendResult, error)
    SendMedia(to string, mediaURL string, mediaBuffer []byte, fileName string, opts *SendOptions) (*SendResult, error)

    // 解析
    ParseMediaKeys(content string, msgType string) *MediaKeys
    ParsePostContent(content string) *PostParseResult
    DetectFileType(fileName string) string
}

type SendOptions struct {
    ReplyToMessageID string
    ReplyInThread    bool
    AccountID        string
}
```

---

## 13. 关键实现注意事项

1. **飞书 `messageResource.get` API 的 type 参数**：只有 `"image"` 和 `"file"` 两种值，所有非图片资源一律用 `"file"`。
2. **视频消息有两个 key**：`file_key`（视频文件）和 `image_key`（缩略图），通常只需下载视频文件。
3. **发送视频的 msg_type 是 `"media"`**（不是 `"video"`），这是飞书 API 的命名。
4. **发送语音的 msg_type 是 `"audio"`**，对应上传的 file_type 是 `"opus"`。
5. **文件名不要做 URL 编码**：飞书 API 会直接显示上传的 file_name，percent-encoding 会导致乱码。
6. **富文本 post 的 content 结构**：是二维数组 `[[element, element], [element]]`，每个内层数组是一个段落。
7. **错误处理**：媒体下载失败不应阻塞消息处理，应记录日志后继续处理下一个媒体。
8. **回复降级**：当原消息已被撤回（错误码 230011/231003），自动降级为直接发送。
