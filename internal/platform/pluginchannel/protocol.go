package pluginchannel

import "encoding/json"

const ProtocolVersion = 1

type InitializeParams struct {
	ProtocolVersion int            `json:"protocol_version"`
	OpenbeeVersion  string         `json:"openbee_version"`
	ChannelID       string         `json:"channel_id"`
	Language        string         `json:"language"`
	BotName         string         `json:"bot_name"`
	Config          map[string]any `json:"config"`
	DataDir         string         `json:"data_dir"`
	MediaDir        string         `json:"media_dir"`
}

type InitializeResult struct {
	ProtocolVersion int `json:"protocol_version"`
}

type SendParams struct {
	SessionKey   string   `json:"session_key"`
	ChatID       string   `json:"chat_id"`
	SessionScope string   `json:"session_scope"`
	Proactive    bool     `json:"proactive"`
	ReplyTo      *ReplyTo `json:"reply_to,omitempty"`
	Content      string   `json:"content,omitempty"`
	Media        *Media   `json:"media,omitempty"`
}

type ReplyTo struct {
	MessageID string `json:"message_id,omitempty"`
	Raw       string `json:"raw,omitempty"`
}

type Media struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type SendResult struct {
	PlatformMessageID string `json:"platform_message_id,omitempty"`
}

type InboundParams struct {
	ChatID       string          `json:"chat_id"`
	SenderID     string          `json:"sender_id"`
	SessionScope string          `json:"session_scope,omitempty"`
	MessageID    string          `json:"message_id,omitempty"`
	Text         string          `json:"text,omitempty"`
	Attachments  []Attachment    `json:"attachments,omitempty"`
	Raw          string          `json:"raw,omitempty"`
	Context      json.RawMessage `json:"context,omitempty"`
	Time         int64           `json:"time,omitempty"`
}

type Attachment struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type LogParams struct {
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}
