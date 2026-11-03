package model

import (
	"strconv"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const maxBadge = 99

func ShouldNotify(ev api.ChatEvent, foreground bool) bool {
	if ev.Type != "message" || ev.Message == nil || foreground {
		return false
	}
	m := ev.Message
	if m.Sender.Type == "user" || m.Kind == "reset" {
		return false
	}
	if m.Sender.Type == "bee" {
		if _, inWorkerChat := api.ConversationWorkerID(ev.ConversationID); inWorkerChat {
			return false
		}
	}
	return true
}

func UnreadTotal(cs []api.ConversationSummary) int {
	total := 0
	for _, c := range cs {
		total += c.Unread
	}
	return total
}

func BadgeText(n int) string {
	switch {
	case n <= 0:
		return ""
	case n > maxBadge:
		return strconv.Itoa(maxBadge) + "+"
	}
	return strconv.Itoa(n)
}

const maxIconBadge = 9

func IconBadgeText(n int) string {
	if n > maxIconBadge {
		return strconv.Itoa(maxIconBadge) + "+"
	}
	return BadgeText(n)
}
