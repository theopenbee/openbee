package bridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
)

const defaultBeeID = "default"

type EnvResolver interface {
	ResolveBeeEnv(string) ([]string, error)
	ResolveWorkerEnv(string) ([]string, error)
}

type SystemConfigReader interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
}

type ServiceOptions struct {
	Engines      EngineSet
	EngineCfg    *enginecfg.Store
	TokenSecret  string
	TokenTTL     time.Duration
	Env          EnvResolver
	SystemConfig SystemConfigReader
}

type Service struct {
	engines      map[string]ai.EngineAdapter
	engineCfg    *enginecfg.Store
	tokenSecret  string
	tokenTTL     time.Duration
	env          EnvResolver
	systemConfig SystemConfigReader
}

func NewService(opts ServiceOptions) *Service {
	engines := opts.Engines.adapters
	if engines == nil {
		engines = map[string]ai.EngineAdapter{}
	}
	return &Service{
		engines:      engines,
		engineCfg:    opts.EngineCfg,
		tokenSecret:  opts.TokenSecret,
		tokenTTL:     opts.TokenTTL,
		env:          opts.Env,
		systemConfig: opts.SystemConfig,
	}
}

func (s *Service) EnabledEngines() []string {
	enabled := make([]string, 0, len(s.engines))
	for _, name := range AllEngines() {
		if _, ok := s.engines[name]; ok {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

func (s *Service) ValidateEngine(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := s.engines[name]; !ok {
		return fmt.Errorf("engine %q is not enabled", name)
	}
	return nil
}

func (s *Service) ResolveEngine(workerEngine string) (string, error) {
	if workerEngine != "" {
		if _, ok := s.engines[workerEngine]; ok {
			return workerEngine, nil
		}
	}
	if s.engineCfg == nil {
		return "", fmt.Errorf("no engine adapter found (worker engine %q, no default configured)", workerEngine)
	}
	defaultEngine := s.engineCfg.Get()
	if _, ok := s.engines[defaultEngine]; ok {
		return defaultEngine, nil
	}
	return "", fmt.Errorf("no engine adapter found (worker engine %q, default %q)", workerEngine, defaultEngine)
}

func (s *Service) BuildBeeSessionPrefix() string {
	return BuildBeeSessionPrefix()
}

func (s *Service) BuildWorkerSessionPrefix(persona WorkerPersona) string {
	return BuildWorkerSessionPrefix(persona)
}

func (s *Service) PrepareBeeWorkspace(workDir string) error {
	for _, name := range AllEngines() {
		adapter, ok := s.engines[name]
		if !ok {
			continue
		}
		if err := adapter.Prepare(workDir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
			return fmt.Errorf("prepare engine %q: %w", name, err)
		}
	}
	return nil
}

func (s *Service) PrepareWorkerWorkspace(workDir string, engineName string) error {
	resolved, err := s.ResolveEngine(engineName)
	if err != nil {
		return err
	}
	if err := s.engines[resolved].Prepare(workDir, ai.PrepareOptions{Role: ai.RoleWorker}); err != nil {
		return fmt.Errorf("prepare engine %q: %w", resolved, err)
	}
	return nil
}

func (s *Service) RunBee(ctx context.Context, req BeeRunRequest) (RunHandle, error) {
	engineName, err := s.ResolveEngine("")
	if err != nil {
		return RunHandle{}, err
	}
	token, err := auth.GenerateBeeToken(s.tokenSecret, s.tokenTTL)
	if err != nil {
		return RunHandle{}, fmt.Errorf("generate bee token: %w", err)
	}
	extraEnv, err := s.resolveBeeEnv(defaultBeeID)
	if err != nil {
		return RunHandle{}, fmt.Errorf("resolve bee env: %w", err)
	}
	runRes, err := s.engines[engineName].Run(ctx, req.WorkDir, req.Prompt, ai.RunOptions{
		SessionID: req.SessionID,
		Resume:    req.Resume,
		APIKey:    token,
		ExtraEnv:  extraEnv,
		ExtraArgs: s.resolveBeeEngineArgs(ctx, engineName),
	}, req.LogPath)
	if err != nil {
		return RunHandle{}, err
	}
	return newRunHandle(engineName, runRes), nil
}

func (s *Service) RunWorker(ctx context.Context, req WorkerRunRequest) (RunHandle, error) {
	engineName, err := s.ResolveEngine(req.WorkerEngine)
	if err != nil {
		return RunHandle{}, err
	}
	token, err := auth.GenerateWorkerToken(s.tokenSecret, req.WorkerID, req.PermissionScopes, s.tokenTTL)
	if err != nil {
		return RunHandle{}, fmt.Errorf("generate worker token: %w", err)
	}
	extraEnv, err := s.resolveWorkerEnv(req.WorkerID)
	if err != nil {
		return RunHandle{}, fmt.Errorf("resolve worker env: %w", err)
	}
	runRes, err := s.engines[engineName].Run(ctx, req.WorkDir, req.Prompt, ai.RunOptions{
		SessionID: req.SessionID,
		Resume:    req.Resume,
		APIKey:    token,
		ExtraEnv:  extraEnv,
		ExtraArgs: s.resolveWorkerEngineArgs(ctx, req.WorkerEngineArgs, engineName),
	}, req.LogPath)
	if err != nil {
		return RunHandle{}, err
	}
	return newRunHandle(engineName, runRes), nil
}

func (s *Service) CollectTokenUsage(ctx context.Context, sessionID, engineName string) (UsageResult, error) {
	if engineName != "" {
		if adapter, ok := s.engines[engineName]; ok {
			return s.collectTokenUsage(ctx, adapter, sessionID, engineName)
		}
	}
	for _, name := range AllEngines() {
		adapter, ok := s.engines[name]
		if !ok {
			continue
		}
		result, err := s.collectTokenUsage(ctx, adapter, sessionID, name)
		if errors.Is(err, ErrSessionDataNotFound) {
			continue
		}
		return result, err
	}
	return UsageResult{}, ErrSessionDataNotFound
}

func (s *Service) collectTokenUsage(ctx context.Context, adapter ai.EngineAdapter, sessionID, engineName string) (UsageResult, error) {
	usages, err := adapter.CollectTokenUsage(ctx, sessionID)
	if errors.Is(err, ai.ErrSessionDataNotFound) {
		return UsageResult{}, ErrSessionDataNotFound
	}
	if err != nil {
		return UsageResult{}, err
	}
	return UsageResult{Engine: engineName, Usages: mapTokenUsages(usages)}, nil
}

func (s *Service) resolveBeeEnv(scopeID string) ([]string, error) {
	if s.env == nil {
		return nil, nil
	}
	return s.env.ResolveBeeEnv(scopeID)
}

func (s *Service) resolveWorkerEnv(workerID string) ([]string, error) {
	if s.env == nil {
		return nil, nil
	}
	return s.env.ResolveWorkerEnv(workerID)
}

func (s *Service) resolveBeeEngineArgs(ctx context.Context, engineName string) []string {
	globalMap := s.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
	beeMap := s.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsBee)
	merged := ai.MergeEngineArgs(globalMap, beeMap)
	return merged[engineName]
}

func (s *Service) resolveWorkerEngineArgs(ctx context.Context, workerEngineArgs, engineName string) []string {
	globalMap := s.loadEngineArgs(ctx, model.SystemConfigKeyEngineArgsGlobal)
	workerMap := ai.ParseEngineArgsJSON(workerEngineArgs)
	merged := ai.MergeEngineArgs(globalMap, workerMap)
	return merged[engineName]
}

func (s *Service) loadEngineArgs(ctx context.Context, key string) ai.EngineArgsMap {
	if s.systemConfig == nil {
		return nil
	}
	cfg, found, err := s.systemConfig.Get(ctx, key)
	if err != nil || !found {
		return nil
	}
	return ai.ParseEngineArgsJSON(cfg.Value)
}

func newRunHandle(engineName string, runRes ai.RunResult) RunHandle {
	return RunHandle{
		Engine:        engineName,
		Process:       runRes.Process,
		Events:        mapEvents(runRes.Output),
		ExtractResult: runRes.ExtractResult,
	}
}

func mapEvents(outputs <-chan ai.Output) <-chan LifecycleEvent {
	events := make(chan LifecycleEvent)
	go func() {
		defer close(events)
		for output := range outputs {
			switch output.Type {
			case ai.OutputDone:
				events <- LifecycleEvent{Type: LifecycleDone, Content: output.Content}
			case ai.OutputError:
				events <- LifecycleEvent{Type: LifecycleError, Content: output.Content}
			}
		}
	}()
	return events
}

func mapTokenUsages(usages []ai.TokenUsage) []TokenUsage {
	mapped := make([]TokenUsage, len(usages))
	for i, usage := range usages {
		mapped[i] = TokenUsage{
			Model:               usage.Model,
			InputTokens:         usage.InputTokens,
			OutputTokens:        usage.OutputTokens,
			CacheCreationTokens: usage.CacheCreationTokens,
			CacheReadTokens:     usage.CacheReadTokens,
		}
	}
	return mapped
}
