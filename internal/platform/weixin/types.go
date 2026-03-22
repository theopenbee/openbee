package weixin

// Message type constants.
const (
	MessageTypeUser = 1
	MessageTypeBot  = 2
)

// Message item type constants.
const (
	MessageItemTypeText  = 1
	MessageItemTypeImage = 2
	MessageItemTypeVoice = 3
	MessageItemTypeFile  = 4
	MessageItemTypeVideo = 5
)

// Message state constants.
const (
	MessageStateNew        = 0
	MessageStateGenerating = 1
	MessageStateFinish     = 2
)

// Typing status constants.
const (
	TypingStatusTyping = 1
	TypingStatusCancel = 2
)

// WeixinMessage represents a message in the WeChat protocol.
type WeixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	ItemList     []MessageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	DeleteTimeMs int64         `json:"delete_time_ms,omitempty"`
}

// MessageItem represents a single content item within a message.
type MessageItem struct {
	Type         int        `json:"type,omitempty"`
	CreateTimeMs int64      `json:"create_time_ms,omitempty"`
	TextItem     *TextItem  `json:"text_item,omitempty"`
	ImageItem    *ImageItem `json:"image_item,omitempty"`
	VoiceItem    *VoiceItem `json:"voice_item,omitempty"`
	FileItem     *FileItem  `json:"file_item,omitempty"`
	VideoItem    *VideoItem `json:"video_item,omitempty"`
}

// TextItem holds text content.
type TextItem struct {
	Text string `json:"text,omitempty"`
}

// ImageItem holds image content with CDN reference.
type ImageItem struct {
	CDNMedia
}

// VoiceItem holds voice content with optional STT text.
type VoiceItem struct {
	CDNMedia
	Text string `json:"text,omitempty"` // speech-to-text result
}

// FileItem holds file content with metadata.
type FileItem struct {
	CDNMedia
	FileName string `json:"file_name,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// VideoItem holds video content with CDN reference.
type VideoItem struct {
	CDNMedia
}

// CDNMedia contains CDN download parameters and encryption key.
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AesKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

// API request/response types.

type GetUpdatesReq struct {
	GetUpdatesBuf string `json:"get_updates_buf,omitempty"`
}

type GetUpdatesResp struct {
	Ret                  int             `json:"ret"`
	ErrCode              int             `json:"errcode"`
	ErrMsg               string          `json:"errmsg,omitempty"`
	Msgs                 []WeixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf        string          `json:"get_updates_buf,omitempty"`
	LongPollingTimeoutMs int64           `json:"longpolling_timeout_ms,omitempty"`
}

type SendMessageReq struct {
	Msg *WeixinMessage `json:"msg,omitempty"`
}

type SendTypingReq struct {
	IlinkUserID  string `json:"ilink_user_id,omitempty"`
	TypingTicket string `json:"typing_ticket,omitempty"`
	Status       int    `json:"status,omitempty"`
}

type GetConfigReq struct {
	IlinkUserID  string `json:"ilink_user_id,omitempty"`
	ContextToken string `json:"context_token,omitempty"`
}

type GetConfigResp struct {
	Ret          int    `json:"ret"`
	TypingTicket string `json:"typing_ticket,omitempty"`
}

type GetUploadUrlReq struct {
	FileKey         string `json:"filekey,omitempty"`
	MediaType       int    `json:"media_type,omitempty"` // 1=image, 2=video, 3=file, 4=voice
	ToUserID        string `json:"to_user_id,omitempty"`
	RawSize         int64  `json:"rawsize,omitempty"`
	RawFileMD5      string `json:"rawfilemd5,omitempty"`
	FileSize        int64  `json:"filesize,omitempty"` // ciphertext size
	ThumbRawSize    int64  `json:"thumb_rawsize,omitempty"`
	ThumbRawFileMD5 string `json:"thumb_rawfilemd5,omitempty"`
	ThumbFileSize   int64  `json:"thumb_filesize,omitempty"`
	AesKey          string `json:"aeskey,omitempty"`
}

type GetUploadUrlResp struct {
	Ret              int    `json:"ret"`
	UploadParam      string `json:"upload_param,omitempty"`
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"`
}

type GetBotQRCodeResp struct {
	Ret    int    `json:"ret"`
	QRCode string `json:"qrcode,omitempty"`
}

type GetQRCodeStatusResp struct {
	Ret      int    `json:"ret"`
	Status   int    `json:"status,omitempty"` // 0=pending, 1=scanned, 2=confirmed
	BotToken string `json:"bot_token,omitempty"`
	BotID    string `json:"ilink_bot_id,omitempty"`
	BaseUrl  string `json:"base_url,omitempty"`
	UserID   string `json:"user_id,omitempty"`
}
