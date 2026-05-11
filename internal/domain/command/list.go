package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// CmdList is the slash command that prints the worker directory.
const CmdList = "/list"

// WorkerLister is the subset of WorkerStore needed by ListCommandHandler.
type WorkerLister interface {
	List() ([]model.Worker, error)
}

// ListCommandHandler handles the /list slash command.
type ListCommandHandler struct {
	workers WorkerLister
	senders map[string]platform.PlatformSenderAdapter
}

func NewListCommandHandler(workers WorkerLister, senders map[string]platform.PlatformSenderAdapter) *ListCommandHandler {
	return &ListCommandHandler{workers: workers, senders: senders}
}

func (h *ListCommandHandler) IsCommand(content string) bool {
	return isExactOrPrefixed(content, CmdList)
}

func (h *ListCommandHandler) HandleCommand(ctx context.Context, content string, replyTo platform.InboundMessage) bool {
	fields := strings.Fields(content)
	if len(fields) == 0 || fields[0] != CmdList {
		return false
	}
	if len(fields) > 2 {
		h.reply(ctx, replyTo, i18n.M.Runtime.ListCommand.Usage)
		return true
	}

	keyword := ""
	if len(fields) == 2 {
		keyword = fields[1]
	}

	workers, err := h.workers.List()
	if err != nil {
		log.Error("list workers for /list", zap.Error(err))
		h.reply(ctx, replyTo, i18n.M.Runtime.ListCommand.LookupFailed)
		return true
	}

	if keyword != "" {
		kw := strings.ToLower(keyword)
		// 3-index slice: zero cap forces a fresh backing array, no aliasing with workers.
		filtered := workers[:0:0]
		for _, w := range workers {
			if strings.Contains(strings.ToLower(w.Description), kw) {
				filtered = append(filtered, w)
			}
		}
		workers = filtered
	}

	sort.SliceStable(workers, func(i, j int) bool { return workers[i].Name < workers[j].Name })
	h.reply(ctx, replyTo, formatList(keyword, workers))
	return true
}

func formatList(keyword string, workers []model.Worker) string {
	m := i18n.M.Runtime.ListCommand
	lines := make([]string, 0, len(workers)+2)
	if keyword == "" {
		lines = append(lines, fmt.Sprintf(m.HeaderAll, len(workers)))
		if len(workers) == 0 {
			lines = append(lines, m.EmptyAll)
		}
	} else {
		lines = append(lines, fmt.Sprintf(m.HeaderSearch, keyword, len(workers)))
		if len(workers) == 0 {
			lines = append(lines, m.EmptySearch)
		}
	}
	for _, w := range workers {
		lines = append(lines, fmt.Sprintf(m.Line, w.Name, statusLabel(w.Status), w.Description))
	}
	return strings.Join(lines, "\n")
}

func statusLabel(s model.WorkerStatus) string {
	m := i18n.M.Runtime.ListCommand
	switch s {
	case model.WorkerStatusIdle:
		return m.StatusIdle
	case model.WorkerStatusWorking:
		return m.StatusWorking
	case model.WorkerStatusError:
		return m.StatusError
	default:
		return string(s)
	}
}

func (h *ListCommandHandler) reply(ctx context.Context, replyTo platform.InboundMessage, text string) {
	sendReply(ctx, h.senders, replyTo, text)
}
