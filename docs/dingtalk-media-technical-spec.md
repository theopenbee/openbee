# 钉钉消息媒体处理技术实现规范

> 供 Golang 重新实现参考，基于 `plugin.ts` 提取的完整技术细节。

---

## 一、整体架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        消息生命周期                              │
│                                                                 │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────────────┐  │
│  │ 接收消息  │ -> │ 内容提取+下载 │ -> │ 发送到 Gateway(LLM) │  │
│  │ (入站)   │    │ (入站处理)    │    │                      │  │
│  └──────────┘    └──────────────┘    └──────────┬───────────┘  │
│                                                  │              │
│                                      ┌───────────▼───────────┐  │
│                                      │   LLM 响应后处理      │  │
│                                      │ (出站媒体处理管线)    │  │
│                                      └───────────┬───────────┘  │
│                                                  │              │
│                              ┌────────────────────▼──────────┐  │
│                              │ 发送回复（AI Card / 普通消息）│  │
│                              │ (出站)                        │  │
│                              └───────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 二、鉴权体系

### 2.1 两套 Access Token

| Token 类型 | 获取方式 | 用途 |
|---|---|---|
| **新版 API Token** | `POST https://api.dingtalk.com/v1.0/oauth2/accessToken` body: `{appKey, appSecret}` | AI Card 创建/更新、主动消息发送、文件下载 |
| **旧版 OAPI Token** | `GET https://oapi.dingtalk.com/gettoken?appkey=&appsecret=` | 媒体上传（图片/视频/音频/文件）、staffId→unionId 转换 |

### 2.2 Token 缓存策略

- 缓存 token 和过期时间
- 提前 60 秒刷新：`if (expiry > now + 60_000)` 则复用缓存
- 新版 API 返回 `expireIn`（秒），乘以 1000 转为毫秒时间戳

---

## 三、入站消息处理（接收）

### 3.1 消息类型解析

`extractMessageContent(data)` 函数按 `data.msgtype` 分发：

| msgtype | 提取逻辑 | 产出 |
|---|---|---|
| `text` | `data.text.content.trim()` | 纯文本 + at 信息 |
| `richText` | 遍历 `data.content.richText[]`，提取 `part.text` 和 `part.pictureUrl` | 文本 + 图片URL列表 |
| `picture` | `data.content.downloadCode` + `data.content.pictureUrl` | `[图片]` 占位 + downloadCodes/imageUrls |
| `audio` | `data.content.recognition` | 语音识别文本，无则 `[语音消息]` |
| `video` | 固定 `[视频]` | — |
| `file` | `data.content.fileName` + `data.content.downloadCode` | `[文件: xxx]` + downloadCodes+fileNames |

**返回结构体**：
```go
type ExtractedMessage struct {
    Text          string
    MessageType   string
    ImageUrls     []string  // 图片直接URL（来自 richText.pictureUrl）
    DownloadCodes []string  // 需通过API下载的 downloadCode
    FileNames     []string  // 与 DownloadCodes 对应的文件名（图片无文件名）
    AtDingtalkIds []string
    AtMobiles     []string
}
```

### 3.2 图片下载

**两种来源**：

1. **直接 URL**（来自 richText 的 pictureUrl）：直接 HTTP GET 下载
2. **downloadCode**（来自 picture 消息）：需先换取 downloadUrl

**downloadCode 换取 URL**：
```
POST https://api.dingtalk.com/v1.0/robot/messageFiles/download
Header: x-acs-dingtalk-access-token: <token>
Body: { downloadCode: "...", robotCode: "<clientId>" }
Response: { downloadUrl: "https://..." }
```

**下载保存路径**：`~/.openclaw/workspace/media/inbound/openclaw-media-<timestamp>-<random>.<ext>`

**扩展名推断**：根据 `Content-Type` header：
- `image/png` → `.png`
- `image/gif` → `.gif`
- `image/webp` → `.webp`
- 其他 → `.jpg`

### 3.3 文件附件下载与内容提取

区分图片和文件的方式：`downloadCodes[i]` 对应有 `fileNames[i]` 的是文件，没有的是图片。

**文件内容提取策略**（按扩展名）：

| 扩展名 | 处理方式 |
|---|---|
| `.txt .md .csv .json .xml .yaml .yml .html .htm .log .conf .ini .sh .py .js .ts .css .sql` | 直接读取 UTF-8 内容，截断到 50000 字符 |
| `.docx` | 用 mammoth 提取纯文本（Go 中可用 `unidoc` 或 `docconv`） |
| `.pdf` | 用 pdf-parse 提取纯文本（Go 中可用 `pdfcpu` 或 `unipdf`） |
| 其他（`.xlsx .pptx .doc .zip` 等） | 仅保存到本地，返回路径提示 |

提取的内容格式：
```
[文件: example.txt]
```内容```
```

保存路径：`~/.openclaw/workspace/media/inbound/<timestamp>-<safeFileName>`

---

## 四、出站媒体后处理管线

LLM 响应文本经过 **4 个后处理阶段**，顺序执行：

```
LLM Response
  → 01. processLocalImages()    // 上传本地图片，替换路径为 media_id
  → 02. processVideoMarkers()   // 提取视频标记，上传+发送视频消息
  → 03. processAudioMarkers()   // 提取音频标记，上传+发送音频消息
  → 04. processFileMarkers()    // 提取文件标记，上传+发送文件消息
  → 最终文本内容（用于 AI Card 或普通消息回复）
```

### 4.1 图片后处理（processLocalImages）

**第一步：Markdown 图片语法匹配**

正则：
```regex
!\[([^\]]*)\]\(((?:file:\/\/\/|MEDIA:|attachment:\/\/\/)[^\)]+|\/(?:tmp|var|private|Users|home|root)[^\)]+|[A-Za-z]:[\\/ ][^\)]+)\)
```

匹配的路径格式：
- `![alt](file:///path/to/image.jpg)`
- `![alt](MEDIA:/path)`
- `![alt](attachment:///path)`
- `![alt](/tmp/xxx.jpg)` `/var/` `/Users/` `/home/` `/root/` 开头
- `![alt](C:\Users\xxx\photo.jpg)` Windows 路径

**第二步：纯文本中的裸路径匹配**

正则：
```regex
`?((?:\/(?:tmp|var|private|Users|home|root)\/[^\s`'",)]+|[A-Za-z]:[\/][^\s`'",)]+)\.(?:png|jpg|jpeg|gif|bmp|webp))`?
```

排除已在 `![](...)` 中的路径（检查前 10 字符是否包含 `](`）。

**路径清理**：
1. 去前缀：`file://` → 空、`MEDIA:` → 空、`attachment://` → 空
2. URL 解码：`decodeURIComponent`（处理中文路径）
3. 去反斜杠转义：`\\ ` → ` `（AI 可能添加）

**上传 API**：
```
POST https://oapi.dingtalk.com/media/upload?access_token=<token>&type=image
Content-Type: multipart/form-data
Field: media = <文件流>
Response: { media_id: "..." }
```

**替换逻辑**：
- Markdown 图片：`![alt](path)` → `![alt](<media_id>)`
- 裸路径：`/tmp/xxx.png` → `![](<media_id>)`

### 4.2 视频后处理（processVideoMarkers）

**标记格式**（由 LLM 在回复末尾输出）：
```
[DINGTALK_VIDEO]{"path":"<本地视频路径>"}[/DINGTALK_VIDEO]
```

正则：`\[DINGTALK_VIDEO\]({.*?})\[\/DINGTALK_VIDEO\]`

**处理流程**：
1. 解析 JSON 提取 `path`
2. 检查文件存在性
3. **提取视频元数据**（用 ffprobe/ffmpeg）：
   - `duration`（秒，取整）
   - `width`、`height`（分辨率）
4. **生成封面截图**（用 ffmpeg）：
   - 截取第 1 秒
   - 尺寸：`?x360`（高度 360px，宽度自动）
   - 保存到临时文件：`<tmpdir>/thumbnail_<timestamp>.jpg`
5. **上传视频**：`type=video`，限制 20MB
6. **上传封面**：`type=image`
7. **发送视频消息**

**被动回复发送**（通过 sessionWebhook）：
```json
{
  "msgtype": "video",
  "video": {
    "duration": "<秒数字符串>",
    "videoMediaId": "<media_id>",
    "videoType": "mp4",
    "picMediaId": "<封面media_id>"
  }
}
```
Header: `x-acs-dingtalk-access-token: <oapiToken>`

**主动发送**（通过 API）：
```
POST https://api.dingtalk.com/v1.0/robot/groupMessages/send  (群聊)
POST https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend  (单聊)
Body: {
  robotCode: "<clientId>",
  openConversationId: "..." 或 userIds: ["..."],
  msgKey: "sampleVideo",
  msgParam: JSON.stringify({
    duration: "<秒数>",
    videoMediaId: "<media_id>",
    videoType: "mp4",
    picMediaId: "<封面media_id>"
  })
}
```

8. 清理临时封面文件
9. 从内容中移除所有视频标记
10. 附加状态信息（`✅ 视频已发送: xxx.mp4`）

### 4.3 音频后处理（processAudioMarkers）

**标记格式**：
```
[DINGTALK_AUDIO]{"path":"<本地音频路径>"}[/DINGTALK_AUDIO]
```

正则：`\[DINGTALK_AUDIO\]({.*?})\[\/DINGTALK_AUDIO\]`

**处理流程**：
1. 解析 JSON 提取 `path`
2. 上传：`type=voice`，限制 20MB
3. **提取音频时长**（用 ffprobe CLI）：
   ```bash
   ffprobe -v quiet -print_format json -show_format <filepath>
   ```
   解析 `format.duration`（浮点秒数），转为毫秒 `Math.floor(sec * 1000)`
4. 发送语音消息

**ffprobe 路径查找优先级**：
1. `@ffprobe-installer/ffprobe` 包（Go 中不适用）
2. 环境变量 `FFPROBE_PATH`
3. 系统 PATH 中的 `ffprobe`

**被动回复发送**：
```json
{
  "msgtype": "voice",
  "voice": {
    "mediaId": "<media_id>",
    "duration": "<毫秒数字符串>"  // 无法获取时默认 "60000"
  }
}
```

**主动发送**：
```json
{
  "robotCode": "<clientId>",
  "msgKey": "sampleAudio",
  "msgParam": "{\"mediaId\":\"...\",\"duration\":\"...\"}"
}
```

### 4.4 文件后处理（processFileMarkers）

**标记格式**：
```
[DINGTALK_FILE]{"path":"<路径>","fileName":"<文件名>","fileType":"<扩展名>"}[/DINGTALK_FILE]
```

正则：`\[DINGTALK_FILE\]({.*?})\[\/DINGTALK_FILE\]`

**处理流程**：
1. 解析 JSON 提取 `path`、`fileName`、`fileType`
2. 检查文件存在性和大小（限制 20MB）
3. 区分音频和普通文件：

**音频文件扩展名**：`mp3 wav amr ogg aac flac m4a`
- 音频文件用 `type=voice` 上传，走音频消息发送流程

**普通文件**：用 `type=file` 上传

**被动回复发送**：
```json
{
  "msgtype": "file",
  "file": {
    "mediaId": "<media_id>",
    "fileName": "<文件名>",
    "fileType": "<扩展名>"
  }
}
```

**主动发送**：
```json
{
  "robotCode": "<clientId>",
  "msgKey": "sampleFile",
  "msgParam": "{\"mediaId\":\"...\",\"fileName\":\"...\",\"fileType\":\"...\"}"
}
```

---

## 五、通用媒体上传函数

`uploadMediaToDingTalk(filePath, mediaType, oapiToken, maxSize, log)`

```
POST https://oapi.dingtalk.com/media/upload?access_token=<token>&type=<mediaType>
Content-Type: multipart/form-data
Field: media = <文件流>, filename=<basename>, contentType=image/jpeg|application/octet-stream
Timeout: 60s
Response: { media_id: "..." }
```

- `mediaType`：`image` | `file` | `video` | `voice`
- `contentType`：image 时为 `image/jpeg`，其他为 `application/octet-stream`
- 上传前检查文件存在性和大小

---

## 六、发送消息体系

### 6.1 两种发送场景

| 场景 | 触发条件 | 特点 |
|---|---|---|
| **被动回复** | 收到用户消息后回复 | 使用 `sessionWebhook`（从回调 data 获取） |
| **主动发送** | AI Card 场景 / asyncMode | 使用钉钉 API endpoint |

### 6.2 被动回复（sessionWebhook）

```
POST <sessionWebhook>
Header: x-acs-dingtalk-access-token: <token>
Body: { msgtype: "text"|"markdown"|"file"|"video"|"voice", ... }
```

### 6.3 主动发送 API

| 目标 | Endpoint |
|---|---|
| 单聊 | `POST https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend` |
| 群聊 | `POST https://api.dingtalk.com/v1.0/robot/groupMessages/send` |

**单聊 Body**：
```json
{
  "robotCode": "<clientId>",
  "userIds": ["<userId>"],
  "msgKey": "sampleText|sampleMarkdown|sampleFile|sampleVideo|sampleAudio",
  "msgParam": "<JSON字符串>"
}
```

**群聊 Body**：
```json
{
  "robotCode": "<clientId>",
  "openConversationId": "<conversationId>",
  "msgKey": "...",
  "msgParam": "<JSON字符串>"
}
```

### 6.4 消息类型 msgKey 映射

| 类型 | msgKey | msgParam 格式 |
|---|---|---|
| 文本 | `sampleText` | `{"content":"..."}` |
| Markdown | `sampleMarkdown` | `{"title":"...","text":"..."}` |
| 链接 | `sampleLink` | 用户自定义 JSON |
| ActionCard | `sampleActionCard` | 用户自定义 JSON |
| 图片 | `sampleImageMsg` | `{"photoURL":"..."}` |
| 文件 | `sampleFile` | `{"mediaId":"...","fileName":"...","fileType":"..."}` |
| 视频 | `sampleVideo` | `{"duration":"...","videoMediaId":"...","videoType":"mp4","picMediaId":"..."}` |
| 音频 | `sampleAudio` | `{"mediaId":"...","duration":"..."}` |

---

## 七、AI Card 流式响应

### 7.1 卡片模板

固定模板 ID：`02fcf2f4-5e02-4a85-b672-46d1f715543e.schema`

### 7.2 卡片状态机

```
PROCESSING(1) → INPUTING(2) → FINISHED(3)
                              FAILED(5)
```

### 7.3 创建 + 投放流程

**Step 1：创建卡片实例**
```
POST https://api.dingtalk.com/v1.0/card/instances
Body: {
  cardTemplateId: "02fcf2f4-5e02-4a85-b672-46d1f715543e.schema",
  outTrackId: "card_<timestamp>_<random>",
  cardData: { cardParamMap: {} },
  callbackType: "STREAM",
  imGroupOpenSpaceModel: { supportForward: true },
  imRobotOpenSpaceModel: { supportForward: true }
}
```

**Step 2：投放卡片**
```
POST https://api.dingtalk.com/v1.0/card/instances/deliver
```

群聊 Body：
```json
{
  "outTrackId": "<cardInstanceId>",
  "userIdType": 1,
  "openSpaceId": "dtv1.card//IM_GROUP.<openConversationId>",
  "imGroupOpenDeliverModel": { "robotCode": "<clientId>" }
}
```

单聊 Body：
```json
{
  "outTrackId": "<cardInstanceId>",
  "userIdType": 1,
  "openSpaceId": "dtv1.card//IM_ROBOT.<userId>",
  "imRobotOpenDeliverModel": { "spaceType": "IM_ROBOT", "robotCode": "<clientId>" }
}
```

### 7.4 流式更新

**切换到 INPUTING 状态**（首次流式更新前）：
```
PUT https://api.dingtalk.com/v1.0/card/instances
Body: {
  outTrackId: "<cardInstanceId>",
  cardData: {
    cardParamMap: {
      flowStatus: "2",
      msgContent: "",
      staticMsgContent: "",
      sys_full_json_obj: "{\"order\":[\"msgContent\"]}"
    }
  }
}
```

**流式内容更新**（节流间隔 300ms）：
```
PUT https://api.dingtalk.com/v1.0/card/streaming
Body: {
  outTrackId: "<cardInstanceId>",
  guid: "<timestamp>_<random>",
  key: "msgContent",
  content: "<当前累积内容>",
  isFull: true,
  isFinalize: false,
  isError: false
}
```

流式过程中实时清理标记（避免用户看到原始标记）：
- 移除 `[DINGTALK_FILE]...[/DINGTALK_FILE]`
- 移除 `[DINGTALK_VIDEO]...[/DINGTALK_VIDEO]`
- 移除 `[DINGTALK_AUDIO]...[/DINGTALK_AUDIO]`

### 7.5 完成卡片

**Step 1：最终流式更新**（`isFinalize: true`）
```
PUT https://api.dingtalk.com/v1.0/card/streaming
Body: { ..., content: "<最终内容>", isFinalize: true }
```

**Step 2：更新状态为 FINISHED**
```
PUT https://api.dingtalk.com/v1.0/card/instances
Body: {
  outTrackId: "<cardInstanceId>",
  cardData: {
    cardParamMap: {
      flowStatus: "3",
      msgContent: "<最终内容>",
      staticMsgContent: "",
      sys_full_json_obj: "{\"order\":[\"msgContent\"]}"
    }
  }
}
```

---

## 八、Markdown 表格修复

`ensureTableBlankLines(text)` 函数确保 Markdown 表格前有空行，否则钉钉无法正确渲染。

**逻辑**：逐行扫描，当检测到：
1. 当前行包含 `|`（像表头）
2. 下一行匹配分隔行正则 `^\s*\|?\s*:?-+:?\s*(\|?\s*:?-+:?\s*)+\|?\s*$`
3. 前一行非空且不是表格行

则在表头前插入一个空行。

---

## 九、表情回复

处理消息时贴"思考中"表情，完成后撤回。

**贴表情**：
```
POST https://api.dingtalk.com/v1.0/robot/emotion/reply
Body: {
  robotCode: "<clientId>",
  openMsgId: "<msgId>",
  openConversationId: "<conversationId>",
  emotionType: 2,
  emotionName: "🤔思考中",
  textEmotion: {
    emotionId: "2659900",
    emotionName: "🤔思考中",
    text: "🤔思考中",
    backgroundId: "im_bg_1"
  }
}
```

**撤回表情**：
```
POST https://api.dingtalk.com/v1.0/robot/emotion/recall
Body: 同上
```

---

## 十、完整消息处理流程（handleDingTalkMessage）

```
1. extractMessageContent(data) → 解析消息类型和内容
2. DM Policy 检查（allowlist 过滤）
3. 构建 SessionContext（会话隔离）
4. 构建 systemPrompts（包含 mediaSystemPrompt）
5. 获取 oapiToken（用于后处理）
6. 下载入站图片到本地文件 → imageLocalPaths[]
7. 下载入站文件附件 → 提取文本内容追加到 userContent
8. 贴 🤔思考中 表情
9. 调用 Gateway SSE 流式接口获取 LLM 响应
10. 后处理管线（4 阶段）
11. 发送最终回复（AI Card 或普通消息）
12. 撤回表情
```

### 10.1 三种运行模式

| 模式 | 条件 | 行为 |
|---|---|---|
| **AI Card 流式** | `createAICard` 成功 | 流式更新卡片，300ms 节流 |
| **异步模式** | `asyncMode=true` | 立即回执 ack，后台处理，主动推送结果 |
| **降级普通消息** | AI Card 失败 | 收集完整响应后一次性发送 markdown/text |

---

## 十一、关键常量

```go
const (
    DingTalkAPI  = "https://api.dingtalk.com"
    DingTalkOAPI = "https://oapi.dingtalk.com"

    AICardTemplateID = "02fcf2f4-5e02-4a85-b672-46d1f715543e.schema"

    MaxFileSize  = 20 * 1024 * 1024  // 20MB
    MaxVideoSize = 20 * 1024 * 1024  // 20MB

    StreamUpdateInterval = 300  // ms
    MessageDedupTTL      = 5 * 60 * 1000  // 5分钟
)
```

---

## 十二、Go 实现建议

### 12.1 依赖映射

| TypeScript 依赖 | Go 替代方案 |
|---|---|
| `axios` | `net/http` 或 `resty` |
| `dingtalk-stream` (DWClient) | 自行实现 WebSocket 或使用钉钉 Go SDK |
| `form-data` (multipart) | `mime/multipart` |
| `fluent-ffmpeg` | `os/exec` 调用 `ffmpeg`/`ffprobe` |
| `mammoth` (.docx) | `github.com/unidoc/unioffice` 或 `github.com/nguyenthenguyen/docx` |
| `pdf-parse` (.pdf) | `github.com/ledongthuc/pdf` 或 `github.com/pdfcpu/pdfcpu` |

### 12.2 并发模型

- 消息去重：用 `sync.Map` 替代 JS Map
- Token 缓存：用 `sync.Mutex` 保护
- 流式更新节流：用 `time.Ticker` 或记录 `lastUpdateTime`

### 12.3 注意事项

1. **HTTP Header 编码**：`X-OpenClaw-Memory-User` 需要 Base64 编码（中文字符不能直接放 HTTP Header）
2. **multipart 上传**：`contentType` 在 image 类型时为 `image/jpeg`，其他为 `application/octet-stream`
3. **SSE 流式解析**：Gateway 返回 `data: {...}\n\n` 格式，`data: [DONE]` 表示结束
4. **裸路径替换**：从后往前替换，避免 index 偏移
5. **cardParamMap 值必须是字符串**：包括 `flowStatus`（`"1"` 不是 `1`）
