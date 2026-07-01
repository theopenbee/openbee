package command

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// workerResolveMessages holds the user-facing replies resolveSingleWorker sends
// for each failure mode. NotFound takes one %q (the name); Duplicate takes %q
// and %s (the name and the newline-joined match list).
type workerResolveMessages struct {
	LookupFailed string
	NotFound     string
	Duplicate    string
}

// resolveSingleWorker looks up exactly one worker by name for worker-scoped
// slash commands (/clear {worker}, /stop {worker}). It replies with the
// caller-supplied message and returns ok=false when the lookup fails, no worker
// matches, or the name is ambiguous.
func resolveSingleWorker(
	ctx context.Context,
	senders map[string]platform.PlatformSenderAdapter,
	workers WorkerNameLookup,
	replyTo platform.InboundMessage,
	name string,
	msgs workerResolveMessages,
) (model.Worker, bool) {
	workersFound, err := workers.ListByName(name)
	if err != nil {
		log.Error("list workers by name", zap.String("name", name), zap.Error(err))
		sendReply(ctx, senders, replyTo, msgs.LookupFailed)
		return model.Worker{}, false
	}
	if len(workersFound) == 0 {
		sendReply(ctx, senders, replyTo, fmt.Sprintf(msgs.NotFound, name))
		return model.Worker{}, false
	}
	if len(workersFound) > 1 {
		lines := make([]string, 0, len(workersFound))
		for _, w := range workersFound {
			lines = append(lines, fmt.Sprintf("  %s (%s)", w.Name, w.ID))
		}
		sendReply(ctx, senders, replyTo, fmt.Sprintf(msgs.Duplicate, name, strings.Join(lines, "\n")))
		return model.Worker{}, false
	}
	return workersFound[0], true
}
