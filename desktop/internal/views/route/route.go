package route

import (
	"net/url"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	PanelConstraints = "constraints"
	PanelPermissions = "permissions"
	PanelEnv         = "env"
	PanelNew         = "new"
)

const (
	ModeWorker = "worker"
	ModeNew    = "new"
)

const (
	ConnectPath = "/connect"
	SetupPath   = "/setup"
	LoginPath   = "/login"
	ChatPath    = "/chat"
	PanelQuery  = "panel"
)

type Nav interface {
	Push(target string)
	Replace(target string)
	Path() string
	Query(name string) string
}

type Panel struct {
	Mode string
	Tab  string
}

func Chat(conversationParam string) string {
	if conversationParam == "" {
		return ChatPath
	}
	return ChatPath + "/" + url.PathEscape(conversationParam)
}

func ChatPanel(conversationParam, panel string) string {
	if panel == "" {
		return Chat(conversationParam)
	}
	return Chat(conversationParam) + "?" + url.Values{PanelQuery: {panel}}.Encode()
}

func ConversationID(param string) string {
	switch {
	case param == "" || strings.Contains(param, ":"):
		return ""
	case param == api.BeeConversation:
		return api.BeeConversation
	}
	return api.WorkerConversation(param)
}

func Param(conversationID string) string {
	if conversationID == api.BeeConversation {
		return api.BeeConversation
	}
	id, _ := api.ConversationWorkerID(conversationID)
	return id
}

func ParsePanel(param, panel string) Panel {
	switch panel {
	case PanelNew:
		return Panel{Mode: ModeNew}
	case PanelConstraints, PanelPermissions, PanelEnv:
		if param == "" || param == api.BeeConversation {
			return Panel{}
		}
		return Panel{Mode: ModeWorker, Tab: panel}
	}
	return Panel{}
}

func IsTabSwitch(from, to Panel) bool {
	return from.Mode == ModeWorker && to.Mode == ModeWorker
}
