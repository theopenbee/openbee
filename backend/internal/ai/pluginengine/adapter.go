package pluginengine

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/plugin"
)

const (
	checkTimeout = 30 * time.Second
	usageTimeout = 60 * time.Second
)

type Spec struct {
	ID             string
	Dir            string
	Command        []string
	Config         map[string]any
	DataDir        string
	StateDir       string
	Language       string
	OpenbeeVersion string
	Runtime        *plugin.RuntimeResolver
}

type lineLogger interface {
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
}

type Adapter struct {
	spec Spec
	log  lineLogger
}

var _ ai.EngineAdapter = (*Adapter)(nil)

func New(spec Spec) *Adapter {
	return &Adapter{spec: spec, log: logger.With(zap.String("component", "engine:"+spec.ID))}
}

func (a *Adapter) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	params := runParams{
		Role:      string(opts.Role),
		WorkDir:   workDir,
		Prompt:    prompt,
		SessionID: opts.SessionID,
		Resume:    opts.Resume,
		ExtraArgs: opts.ExtraArgs,
		LogPath:   logPath,
	}
	if params.ExtraArgs == nil {
		params.ExtraArgs = []string{}
	}
	if url := ai.MCPURL(); url != "" {
		params.MCP = &mcpParams{Name: ai.MCPServerName, URL: url, APIKeyEnv: ai.APIKeyEnv}
	}
	p, err := a.start(ctx, methodRun, params, func(runtimeEnv []string) []string {
		return ai.BuildRunEnv(append(ai.BuildBaseEnv(), runtimeEnv...), opts.ExtraEnv, opts.APIKey)
	})
	if err != nil {
		return ai.RunResult{}, err
	}

	var text atomic.Pointer[string]
	ch := make(chan ai.Output, 1)
	go func() {
		defer close(ch)
		waitErr := p.wait()
		out, result := runOutcome(waitErr, p.resp, p.lastStderr)
		text.Store(&result)
		ch <- out
	}()
	return ai.NewRunResult(p.proc, ch, nil, func(string) string {
		if s := text.Load(); s != nil {
			return *s
		}
		return ""
	})
}

func runOutcome(waitErr error, resp *response, lastStderr string) (ai.Output, string) {
	if resp == nil {
		return failed(noResponseError(waitErr, lastStderr).Error()), ""
	}
	if resp.Err != "" {
		return failed(resp.Err), ""
	}
	var res runResult
	if err := decodeResult(resp.Result, &res); err != nil {
		return failed(fmt.Sprintf("decode engine run result: %v", err)), ""
	}
	switch {
	case res.Error != "":
		return failed(res.Error), res.Text
	case waitErr != nil:
		return failed(waitErr.Error()), res.Text
	}
	return ai.Output{Type: ai.OutputDone}, res.Text
}

func failed(msg string) ai.Output {
	return ai.Output{Type: ai.OutputError, Content: msg}
}

func (a *Adapter) CollectTokenUsage(ctx context.Context, sessionIDs []string) (map[string][]ai.TokenUsage, error) {
	usage := map[string][]ai.TokenUsage{}
	if len(sessionIDs) == 0 {
		return usage, nil
	}
	var res usageResult
	if err := a.call(ctx, methodUsage, usageParams{SessionIDs: sessionIDs}, usageTimeout, &res); err != nil {
		return nil, err
	}
	for id, entries := range res.Usage {
		list := make([]ai.TokenUsage, 0, len(entries))
		for _, e := range entries {
			list = append(list, ai.TokenUsage{
				Model:               e.Model,
				InputTokens:         e.InputTokens,
				OutputTokens:        e.OutputTokens,
				CacheCreationTokens: e.CacheCreationTokens,
				CacheReadTokens:     e.CacheReadTokens,
			})
		}
		usage[id] = list
	}
	return usage, nil
}

func (a *Adapter) Check(ctx context.Context) (CheckResult, error) {
	var res CheckResult
	if err := a.call(ctx, methodCheck, struct{}{}, checkTimeout, &res); err != nil {
		return CheckResult{}, err
	}
	return res, nil
}

func (a *Adapter) call(ctx context.Context, method string, params any, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	p, err := a.start(ctx, method, params, func(runtimeEnv []string) []string {
		return withoutEnv(append(ai.BuildBaseEnv(), runtimeEnv...), ai.APIKeyEnv)
	})
	if err != nil {
		return err
	}
	waitErr := p.wait()
	switch {
	case p.resp == nil && ctx.Err() != nil:
		return fmt.Errorf("engine plugin %s: %w", method, ctx.Err())
	case p.resp == nil:
		return noResponseError(waitErr, p.lastStderr)
	case p.resp.Err != "":
		return errors.New(p.resp.Err)
	case waitErr != nil:
		return waitErr
	}
	if err := decodeResult(p.resp.Result, out); err != nil {
		return fmt.Errorf("decode engine %s result: %w", method, err)
	}
	return nil
}
