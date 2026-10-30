package model

import (
	"strings"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const GroupGap = 5 * time.Minute

const handoffPreviewLines = 3

type StatusKind int

const (
	StatusIdle StatusKind = iota
	StatusError
	StatusRunning
	StatusQueued
)

func StartsGroup(msgs []Message, i int) bool {
	if i < 0 || i >= len(msgs) {
		return false
	}
	if i == 0 {
		return true
	}
	prev, m := msgs[i-1], msgs[i]
	return prev.Kind == "reset" ||
		m.Kind == "reset" ||
		prev.Sender != m.Sender ||
		m.TS-prev.TS >= GroupGap.Milliseconds()
}

func ChatStatus(a api.ConversationActivity, worker *api.Worker) (StatusKind, int) {
	switch a.State {
	case "queued":
		return StatusQueued, a.QueuedAhead
	case "running":
		return StatusRunning, 0
	}
	if worker != nil && worker.Status == "error" {
		return StatusError, 0
	}
	return StatusIdle, 0
}

func HandoffPreview(content string) string {
	lines := make([]string, 0, handoffPreviewLines)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) == handoffPreviewLines {
			break
		}
	}
	return strings.Join(lines, "\n")
}
