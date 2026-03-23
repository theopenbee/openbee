# @tencent-weixin/openclaw-weixin 技术规格文档

## 1. 概述

**功能：** 微信（WeChat）消息通道插件 —— 通过 HTTP JSON API + 长轮询 + AES-128-ECB 加密，实现与微信平台的消息收发。

**核心能力：**
- 多账号管理 & QR 码登录
- 长轮询消息接收
- 文本/图片/视频/文件/语音消息收发
- CDN 加密上传/下载
- 会话超时恢复

---

## 2. API 端点

**默认 Base URL：** `https://ilinkai.weixin.qq.com`
**默认 CDN URL：** `https://novac2c.cdn.weixin.qq.com/c2c`

### 2.1 公共请求头

| Header | Value |
|--------|-------|
| `Content-Type` | `application/json` |
| `AuthorizationType` | `ilink_bot_token`（固定值） |
| `Authorization` | `Bearer {token}` |
| `X-WECHAT-UIN` | 随机 uint32 → string → base64 |
| `SKRouteTag` | `{routeTag}`（可选，来自配置） |

所有请求 body 包含：
```json
{ "base_info": { "channel_version": "1.0.2" }, ...其他字段 }
```

### 2.2 端点列表

#### POST `ilink/bot/getupdates` — 长轮询获取消息
**请求：**
```json
{
  "get_updates_buf": "<sync_cursor>",
  "base_info": { "channel_version": "1.0.2" }
}
```
**响应：**
```json
{
  "ret": 0,
  "msgs": [WeixinMessage],
  "get_updates_buf": "<new_cursor>",
  "longpolling_timeout_ms": 35000
}
```
- 超时 35s（客户端），服务端可保持连接
- 错误码 `-14` = 会话过期，触发 1 小时暂停

#### POST `ilink/bot/sendmessage` — 发送消息
**请求：**
```json
{
  "msg": WeixinMessage,
  "base_info": { "channel_version": "1.0.2" }
}
```
超时 15s。

#### POST `ilink/bot/getuploadurl` — 获取 CDN 上传地址
**请求：**
```json
{
  "filekey": "<random_hex_32>",
  "media_type": 1|2|3|4,
  "to_user_id": "<target>",
  "rawsize": "<明文字节数>",
  "rawfilemd5": "<hex_md5>",
  "filesize": "<密文字节数>",
  "thumb_rawsize": "<可选>",
  "thumb_rawfilemd5": "<可选>",
  "thumb_filesize": "<可选>",
  "aeskey": "<hex_32_bytes>"
}
```
**响应：**
```json
{
  "upload_param": "<encrypted_params>",
  "thumb_upload_param": "<encrypted_params>"
}
```
超时 15s。`media_type`: 1=图片, 2=视频, 3=文件, 4=语音。

#### POST `ilink/bot/getconfig` — 获取账号配置
**请求：**
```json
{
  "ilink_user_id": "<user_id>",
  "context_token": "<可选>",
  "base_info": { "channel_version": "1.0.2" }
}
```
**响应：**
```json
{ "ret": 0, "typing_ticket": "<base64_bytes>" }
```
超时 10s。缓存 24 小时，失败时指数退避（2s → 1h）。

#### POST `ilink/bot/sendtyping` — 发送输入状态
**请求：**
```json
{
  "ilink_user_id": "<user_id>",
  "typing_ticket": "<from_getconfig>",
  "status": 1|2,
  "base_info": { "channel_version": "1.0.2" }
}
```
超时 10s。`status`: 1=正在输入, 2=取消。

#### GET `ilink/bot/get_bot_qrcode?bot_type=3` — 获取登录二维码
**响应：**
```json
{
  "qrcode": "<uuid>",
  "qrcode_img_content": "<data_url_or_html>"
}
```

#### GET `ilink/bot/get_qrcode_status?qrcode=<uuid>` — 轮询二维码状态
**额外请求头：** `iLink-App-ClientVersion: 1`
**响应：**
```json
{
  "status": "wait|scaned|confirmed|expired",
  "bot_token": "<token>",
  "ilink_bot_id": "<bot_id>",
  "baseurl": "<api_base>",
  "ilink_user_id": "<user_id>"
}
```
超时 35s（长轮询）。

---

## 3. 数据模型

### 3.1 WeixinMessage
```go
type WeixinMessage struct {
    Seq          int64          `json:"seq,omitempty"`
    MessageID    int64          `json:"message_id,omitempty"`
    FromUserID   string         `json:"from_user_id,omitempty"`
    ToUserID     string         `json:"to_user_id,omitempty"`
    ClientID     string         `json:"client_id,omitempty"`
    CreateTimeMs int64          `json:"create_time_ms,omitempty"`
    UpdateTimeMs int64          `json:"update_time_ms,omitempty"`
    DeleteTimeMs int64          `json:"delete_time_ms,omitempty"`
    SessionID    string         `json:"session_id,omitempty"`
    GroupID      string         `json:"group_id,omitempty"`
    MessageType  int            `json:"message_type,omitempty"`  // 1=USER, 2=BOT
    MessageState int            `json:"message_state,omitempty"` // 0=NEW, 1=GENERATING, 2=FINISH
    ItemList     []MessageItem  `json:"item_list,omitempty"`
    ContextToken string         `json:"context_token,omitempty"`
}
```

### 3.2 MessageItem
```go
type MessageItem struct {
    Type         int        `json:"type,omitempty"`    // 1=TEXT, 2=IMAGE, 3=VOICE, 4=FILE, 5=VIDEO
    CreateTimeMs int64      `json:"create_time_ms,omitempty"`
    UpdateTimeMs int64      `json:"update_time_ms,omitempty"`
    IsCompleted  bool       `json:"is_completed,omitempty"`
    MsgID        string     `json:"msg_id,omitempty"`
    RefMsg       *RefMessage `json:"ref_msg,omitempty"`
    TextItem     *TextItem  `json:"text_item,omitempty"`
    ImageItem    *ImageItem `json:"image_item,omitempty"`
    VoiceItem    *VoiceItem `json:"voice_item,omitempty"`
    FileItem     *FileItem  `json:"file_item,omitempty"`
    VideoItem    *VideoItem `json:"video_item,omitempty"`
}

type TextItem struct {
    Text string `json:"text,omitempty"`
}
```

### 3.3 CDNMedia
```go
type CDNMedia struct {
    EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
    AesKey            string `json:"aes_key,omitempty"` // base64 编码，两种格式见下文
    EncryptType       int    `json:"encrypt_type,omitempty"`
}
```

### 3.4 媒体项
```go
type ImageItem struct {
    Media       *CDNMedia `json:"media,omitempty"`
    ThumbMedia  *CDNMedia `json:"thumb_media,omitempty"`
    AesKey      string    `json:"aeskey,omitempty"` // hex 字符串，优先于 media.aes_key
    URL         string    `json:"url,omitempty"`
    MidSize     int64     `json:"mid_size,omitempty"`
    ThumbSize   int64     `json:"thumb_size,omitempty"`
    ThumbHeight int       `json:"thumb_height,omitempty"`
    ThumbWidth  int       `json:"thumb_width,omitempty"`
    HDSize      int64     `json:"hd_size,omitempty"`
}

type VoiceItem struct {
    Media         *CDNMedia `json:"media,omitempty"`
    EncodeType    int       `json:"encode_type,omitempty"` // 6=SILK, 7=MP3 等
    BitsPerSample int      `json:"bits_per_sample,omitempty"`
    SampleRate    int      `json:"sample_rate,omitempty"`
    Playtime      int      `json:"playtime,omitempty"` // 毫秒
    Text          string   `json:"text,omitempty"`     // ASR 文本
}

type FileItem struct {
    Media    *CDNMedia `json:"media,omitempty"`
    FileName string    `json:"file_name,omitempty"`
    MD5      string    `json:"md5,omitempty"`
    Len      string    `json:"len,omitempty"` // 明文大小，字符串类型
}

type VideoItem struct {
    Media       *CDNMedia `json:"media,omitempty"`
    VideoSize   int64     `json:"video_size,omitempty"`
    PlayLength  int       `json:"play_length,omitempty"` // 毫秒
    VideoMD5    string    `json:"video_md5,omitempty"`
    ThumbMedia  *CDNMedia `json:"thumb_media,omitempty"`
    ThumbSize   int64     `json:"thumb_size,omitempty"`
    ThumbHeight int       `json:"thumb_height,omitempty"`
    ThumbWidth  int       `json:"thumb_width,omitempty"`
}
```

---

## 4. 加密（AES-128-ECB）

**算法：** AES-128，ECB 模式，PKCS7 填充

**密文大小计算：**
```go
ciphertextSize = ((plaintextSize + 1) / 16 + 1) * 16
// 即 math.Ceil((plaintextSize+1) / 16) * 16
```

### 4.1 加密（上传文件）
1. 生成随机 16 字节 AES key
2. 用 PKCS7 填充明文到 16 字节对齐
3. AES-128-ECB 加密
4. 上传密文到 CDN
5. 在消息 `aes_key` 字段存储 key（base64 编码）

### 4.2 解密（下载文件）
1. 从 CDN 下载密文
2. 解析 `CDNMedia.aes_key`（base64 解码后）：
   - 若为 16 字节 → 直接作为 AES key
   - 若为 32 字节 ASCII hex 字符串 → hex decode 得到 16 字节 key
3. AES-128-ECB 解密 + 去除 PKCS7 填充

---

## 5. 文件上传流程

```
1. 读取文件 → 明文 buffer
2. 计算:
   - rawsize = len(plaintext)
   - rawfilemd5 = md5(plaintext).Hex()
   - filesize = aesEcbPaddedSize(rawsize)
   - filekey = randomBytes(16).Hex()  // 32 字符
   - aeskey = randomBytes(16)

3. POST getuploadurl → 获得 upload_param

4. 加密并上传:
   - ciphertext = encryptAesEcb(plaintext, aeskey)
   - PUT {cdnBaseUrl}/upload?encrypted_query_param={upload_param}
   - 响应头 x-encrypted-param → downloadEncryptedQueryParam

5. 构建消息:
   - CDNMedia.encrypt_query_param = downloadEncryptedQueryParam
   - CDNMedia.aes_key = base64(aeskey)
   - 放入 item_list → sendmessage
```

**重试策略：** 最多 3 次，仅对 5xx 重试，4xx 立即失败。

---

## 6. 文件下载流程

```
1. 从消息中提取 encrypt_query_param 和 aes_key
2. URL = cdnBaseUrl + "/download?encrypted_query_param=" + urlEncode(param)
3. HTTP GET 下载密文
4. 解析 AES key（双格式处理）
5. decryptAesEcb(ciphertext, key)
6. 保存明文文件
```

**限制：** 单文件最大 100MB。

---

## 7. 认证 & 会话管理

### 7.1 QR 码登录流程
1. `GET get_bot_qrcode?bot_type=3` → 获取二维码 UUID
2. 展示二维码给用户扫描
3. 长轮询 `GET get_qrcode_status?qrcode={uuid}` (35s 超时)
4. 状态：`wait` → `scaned` → `confirmed` / `expired`
5. `confirmed` 时获得 `bot_token` + `ilink_bot_id`
6. 保存 token 到账号文件

### 7.2 Token 存储
```
~/.openclaw/openclaw-weixin/accounts/{accountId}.json
```
```json
{
  "token": "bearer_token",
  "savedAt": "2026-03-22T12:00:00.000Z",
  "baseUrl": "https://ilinkai.weixin.qq.com",
  "userId": "scanner_user_id"
}
```
文件权限：`0600`

### 7.3 会话过期恢复
- 错误码 `-14` → 暂停该账号所有 API 调用 **60 分钟**
- 自动恢复或用户重新扫码

### 7.4 Context Token（关键！）
- 每条入站消息携带 `context_token`
- **必须**在回复消息中回传此 token
- 缺失会导致对话关联断裂
- 存储：内存 map，key 为 `accountId:userId`

---

## 8. 消息路由

### 8.1 出站媒体路由（按 MIME 类型）
| MIME | media_type | 操作 |
|------|-----------|------|
| `video/*` | 2 | uploadVideo → sendVideoMessage |
| `image/*` | 1 | uploadFile → sendImageMessage |
| 其他 | 3 | uploadFileAttachment → sendFileMessage |

### 8.2 入站媒体提取优先级
1. Image → 2. Video → 3. File → 4. Voice

---

## 9. 常量

```go
const (
    DefaultBaseURL   = "https://ilinkai.weixin.qq.com"
    DefaultCDNURL    = "https://novac2c.cdn.weixin.qq.com/c2c"
    ChannelVersion   = "1.0.2"

    // 超时
    LongPollTimeoutMs    = 35000
    APITimeoutMs         = 15000
    ConfigTimeoutMs      = 10000
    QRPollTimeoutMs      = 35000
    SessionPauseDuration = 60 * time.Minute
    ConfigCacheTTL       = 24 * time.Hour

    // 重试
    MaxConsecutiveFailures = 3
    BackoffDelay          = 30 * time.Second
    RetryDelay            = 2 * time.Second
    UploadMaxRetries      = 3
    ConfigInitialRetry    = 2 * time.Second
    ConfigMaxRetry        = 1 * time.Hour

    // 限制
    MaxMediaBytes     = 100 * 1024 * 1024
    MaxQRRefreshCount = 3

    // 消息类型枚举
    MessageTypeUser = 1
    MessageTypeBot  = 2

    MessageStateNew        = 0
    MessageStateGenerating = 1
    MessageStateFinish     = 2

    ItemTypeText  = 1
    ItemTypeImage = 2
    ItemTypeVoice = 3
    ItemTypeFile  = 4
    ItemTypeVideo = 5

    UploadMediaImage = 1
    UploadMediaVideo = 2
    UploadMediaFile  = 3
    UploadMediaVoice = 4

    TypingStart  = 1
    TypingCancel = 2
)
```

---

## 10. 多账号管理

- **账号索引：** `~/.openclaw/openclaw-weixin/accounts.json` (ID 数组)
- **账号文件：** `~/.openclaw/openclaw-weixin/accounts/{id}.json`
- **同步游标：** `~/.openclaw/openclaw-weixin/accounts/{id}.sync.json`
- **ID 归一化：** `@` 替换为 `-`（如 `abc@im.bot` → `abc-im-bot`）
- **状态目录：** 环境变量 `OPENCLAW_STATE_DIR` > `CLAWDBOT_STATE_DIR` > `~/.openclaw`

---

## 11. 错误处理

| 错误码 | 含义 | 处理 |
|--------|------|------|
| `0` | 成功 | 正常处理 |
| `-1` | 通用错误 | 带退避重试 |
| `-14` | 会话过期 | 暂停 1 小时 |

**长轮询超时** → 正常现象，返回空结果继续轮询
**CDN 上传失败** → 5xx 重试 3 次，4xx 立即失败
**配置获取失败** → 指数退避 2s→4s→8s→...→1h

---

## 12. 关键实现注意事项

1. **Context Token 必传** — 回复消息必须包含入站消息的 context_token
2. **AES Key 双格式** — base64 解码后可能是 16 字节原始 key 或 32 字符 hex 字符串
3. **Sync Buffer 持久化** — `get_updates_buf` 是不透明 token，原样存储和回传
4. **长轮询超时是正常的** — 不要将超时视为错误
5. **错误通知是尽力而为** — 发送错误消息失败不阻塞主流程
6. **文件权限** — token 文件必须 0600
7. **日志脱敏** — token 只显示前 6 位，URL 去掉 query 参数

---

## 13. 消息流程图

### 13.1 入站消息处理

```
getUpdates (长轮询)
  ↓
收到 WeixinMessage 数组
  ↓
逐条处理:
  ├─ 提取文本内容
  ├─ 检查斜杠命令 (/echo, /toggle-debug)
  │  ├─ 处理并返回（跳过 AI）
  │  └─ 未匹配则继续
  ├─ 下载媒体（如有）:
  │  ├─ 从 CDN 获取
  │  ├─ AES-128-ECB 解密
  │  ├─ 后处理（SILK→WAV 转码）
  │  └─ 保存到本地
  ├─ 转换为内部消息格式
  ├─ 分发到 AI 处理
  ├─ 获取 AI 回复
  ├─ 通过 sendMessage 发送回复（附带 context_token）
  └─ 继续轮询
```

### 13.2 出站媒体发送

```
sendMedia(accountId, to, mediaPath, text)
  ↓
检测 MIME 类型
  ↓
按 MIME 路由:
  ├─ video/*:
  │  ├─ uploadVideoToWeixin()
  │  │  ├─ 生成随机 filekey & aeskey
  │  │  ├─ 计算 hash 和填充大小
  │  │  ├─ getUploadUrl() → upload_param
  │  │  ├─ encryptAesEcb() → 密文
  │  │  ├─ PUT 到 CDN
  │  │  └─ 返回 downloadEncryptedQueryParam & aeskey
  │  └─ sendVideoMessage()
  │
  ├─ image/*:
  │  ├─ uploadFileToWeixin() (media_type=1)
  │  └─ sendImageMessage()
  │
  └─ 其他:
     ├─ uploadFileAttachmentToWeixin() (media_type=3)
     └─ sendFileMessage()
       ↓
构建 MessageItem（含 CDNMedia 引用）
  ↓
获取 context_token
  ↓
sendMessage()
```

---

## 14. CDN 上传详细协议

### 14.1 上传请求

**方法：** PUT
**URL：** `{cdnBaseUrl}/upload?encrypted_query_param={upload_param}`
**Body：** 加密后的文件二进制数据
**Content-Type：** `application/octet-stream`

### 14.2 上传响应

**关键响应头：**
- `x-encrypted-param` — 用于后续下载的加密参数

### 14.3 缩略图上传

对于图片和视频，可能需要单独上传缩略图：
- 使用 `thumb_upload_param` 作为上传参数
- 缩略图也需要 AES 加密
- 响应头中的 `x-encrypted-param` 用于缩略图的下载参数

---

## 15. 配置结构

### 15.1 全局配置（openclaw.json）
```json
{
  "channels": {
    "openclaw-weixin": {
      "baseUrl": "https://ilinkai.weixin.qq.com",
      "cdnBaseUrl": "https://novac2c.cdn.weixin.qq.com/c2c",
      "routeTag": 12345,
      "accounts": {
        "account-id-1": {
          "name": "My Account",
          "enabled": true,
          "baseUrl": "https://...",
          "cdnBaseUrl": "https://...",
          "routeTag": 456
        }
      },
      "logUploadUrl": "https://example.com/upload"
    }
  }
}
```

### 15.2 Route Tag 解析顺序
1. 每账号级 `accounts[accountId].routeTag`
2. 通道级 `channels.openclaw-weixin.routeTag`
3. 不包含（若均未配置）
