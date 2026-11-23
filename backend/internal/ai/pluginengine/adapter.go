package pluginengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/plugin"
)

const (
	checkTimeout      = 30 * time.Second
	usageTimeout      = 60 * time.Second
	modelsTimeout     = 30 * time.Second
	transcriptTimeout = 30 * time.Second
	stopGrace         = 3 * time.Second
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
	spec  Spec
	log   lineLogger
	grace time.Duration
}

var (
	_ ai.EngineAdapter    = (*Adapter)(nil)
	_ ai.ModelLister      = (*Adapter)(nil)
	_ ai.TranscriptReader = (*Adapter)(nil)
)

func New(spec Spec) *Adapter {
	return &Adapter{spec: spec, log: logger.With(zap.String("component", "engine:"+spec.ID)), grace: stopGrace}
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
		Model:     opts.Model,
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
		defer p.finish()
		defer close(ch)
		waitErr := p.wait()
		out, result := runOutcome(waitErr, p.resp, p.lastStderr)
		if out.Type != ai.OutputDone {
			p.sweep()
		}
		text.Store(&result)
		ch <- out
	}()
	return ai.NewRunResult(p, ch, nil, func(string) string {
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
	if err := a.call(ctx, methodUsage, usageParams{SessionIDs: sessionIDs}, nil, usageTimeout, &res); err != nil {
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
	if err := a.call(ctx, methodCheck, struct{}{}, nil, checkTimeout, &res); err != nil {
		return CheckResult{}, err
	}
	return res, nil
}

func (a *Adapter) Models(ctx context.Context, extraEnv []string) ([]ai.ModelInfo, error) {
	var res modelsResult
	if err := a.call(ctx, methodModels, struct{}{}, extraEnv, modelsTimeout, &res); err != nil {
		return nil, err
	}
	models := make([]ai.ModelInfo, 0, len(res.Models))
	for _, m := range res.Models {
		if m.ID == "" {
			continue
		}
		models = append(models, ai.ModelInfo{ID: m.ID, Name: m.Name})
	}
	return models, nil
}

func (a *Adapter) Transcript(ctx context.Context, logPath string) ([]ai.TranscriptStep, error) {
	var res transcriptResult
	if err := a.call(ctx, methodTranscript, transcriptParams{LogPath: logPath}, nil, transcriptTimeout, &res); err != nil {
		if strings.Contains(err.Error(), "unknown method") {
			return nil, fmt.Errorf("%w: %w", ai.ErrTranscriptUnsupported, err)
		}
		return nil, err
	}
	if res.Steps == nil {
		return []ai.TranscriptStep{}, nil
	}
	return res.Steps, nil
}

func (a *Adapter) call(ctx context.Context, method string, params any, extraEnv []string, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	p, err := a.start(ctx, method, params, func(runtimeEnv []string) []string {
		env := append(append(ai.BuildBaseEnv(), runtimeEnv...), extraEnv...)
		return withoutEnv(env, ai.APIKeyEnv)
	})
	if err != nil {
		return err
	}
	defer func() { go p.finish() }()
	waitErr := p.wait()
	if err := callOutcome(ctx, method, waitErr, p.resp, p.lastStderr, out); err != nil {
		p.sweep()
		return err
	}
	return nil
}

func callOutcome(ctx context.Context, method string, waitErr error, resp *response, lastStderr string, out any) error {
	switch {
	case resp == nil && ctx.Err() != nil:
		return fmt.Errorf("engine plugin %s: %w", method, ctx.Err())
	case resp == nil:
		return noResponseError(waitErr, lastStderr)
	case resp.Err != "":
		return errors.New(resp.Err)
	case waitErr != nil:
		return waitErr
	}
	if err := decodeResult(resp.Result, out); err != nil {
		return fmt.Errorf("decode engine %s result: %w", method, err)
	}
	return nil
}
