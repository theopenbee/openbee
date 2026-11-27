package command

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/platform"
)

const (
	minSuggestPrefix   = 2
	maxSuggestDistance = 2
)

var (
	Commands     = []string{CmdClear, CmdStop, CmdStatus, CmdList, CmdCancel, CmdEngine}
	commandToken = regexp.MustCompile(`^/[A-Za-z][A-Za-z0-9_-]{0,31}$`)
)

type UnknownCommandHandler struct {
	senders platform.Senders
}

func NewUnknownCommandHandler(senders platform.Senders) *UnknownCommandHandler {
	return &UnknownCommandHandler{senders: senders}
}

func (h *UnknownCommandHandler) IsCommand(content string) bool {
	_, ok := unknownCommand(content)
	return ok
}

func (h *UnknownCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	token, ok := unknownCommand(content)
	if !ok {
		return false
	}
	m := i18n.M.Runtime.UnknownCommand
	text := fmt.Sprintf(m.Unknown, token)
	if s := suggestCommand(token); s != "" {
		text += fmt.Sprintf(m.Suggestion, s)
	}
	text += "\n" + fmt.Sprintf(m.Available, strings.Join(Commands, " "))
	sendReply(ctx, h.senders, replyTo, text)
	return true
}

func unknownCommand(content string) (string, bool) {
	if !strings.HasPrefix(content, "/") {
		return "", false
	}
	token := strings.Fields(content)[0]
	if !commandToken.MatchString(token) || slices.Contains(Commands, token) {
		return "", false
	}
	return token, true
}

func suggestCommand(token string) string {
	word := strings.ToLower(strings.TrimPrefix(token, "/"))
	var prefixed []string
	for _, cmd := range Commands {
		if len(word) >= minSuggestPrefix && strings.HasPrefix(cmd[1:], word) {
			prefixed = append(prefixed, cmd)
		}
	}
	if len(prefixed) == 1 {
		return prefixed[0]
	}
	best, bestDist := "", maxSuggestDistance+1
	for _, cmd := range Commands {
		if d := editDistance(word, cmd[1:]); d < bestDist {
			best, bestDist = cmd, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
