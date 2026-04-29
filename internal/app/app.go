package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/routes"
	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	_ "github.com/theopenbee/openbee/internal/ai/claude"
	_ "github.com/theopenbee/openbee/internal/ai/codex"
	_ "github.com/theopenbee/openbee/internal/ai/kimi"
	_ "github.com/theopenbee/openbee/internal/ai/pi"
	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/mcp"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/dingtalk"
	"github.com/theopenbee/openbee/internal/platform/feishu"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/platform/telegram"
	"github.com/theopenbee/openbee/internal/platform/wecom"
	"github.com/theopenbee/openbee/internal/platform/weixin"
	"github.com/theopenbee/openbee/internal/tokenstat"
	webui "github.com/theopenbee/openbee/web"
)

// App holds all wired-up components and runs the server.
type App struct {
	db      *sql.DB
	server  *routes.Server
	runners []func(ctx context.Context)
	addr    string
}

// Run starts all goroutines, waits for a signal, then shuts down gracefully.
func (a *App) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, r := range a.runners {
		r := r
		go r(ctx)
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("OpenBee Core starting", zap.String("addr", a.addr))
		if err := a.server.Run(a.addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("Shutting down...")
	case err := <-serverErr:
		logger.Error("server error", zap.Error(err))
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}

	a.db.Close()
}

// BuildApp wires all components together. Returns a ready-to-run App.
func BuildApp(cfg config.Config) (*App, error) {
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	db, s, err := buildStores(cfg.Database)
	if err != nil {
		return nil, err
	}

	engines, err := buildAllEngines(cfg.Bee)
	if err != nil {
		return nil, fmt.Errorf("init engines: %w", err)
	}
	defaultEngine := cfg.Bee.EffectiveEngine()
	if engines[defaultEngine] == nil {
		return nil, fmt.Errorf("default engine %q is not enabled; enable it under bee.engines in config", defaultEngine)
	}

	// Initialize the default engine store from DB, falling back to config.
	engineCfg := enginecfg.NewStore(defaultEngine)
	dbCfg, found, dbErr := s.systemConfigStore.Get(context.Background(), model.SystemConfigKeyDefaultEngine)
	if dbErr != nil {
		logger.Warn("failed to load default engine from DB, falling back to config", zap.Error(dbErr))
	} else if found {
		if engines[dbCfg.Value] != nil {
			engineCfg.Set(dbCfg.Value)
		} else {
			logger.Warn("DB default engine is not enabled, falling back to config",
				zap.String("db_value", dbCfg.Value))
		}
	}

	envSvc, err := env.NewService(s.envConfigStore, s.departmentStore, cfg.Server.EnvSecret)
	if err != nil {
		return nil, fmt.Errorf("init env service: %w", err)
	}
	mgr := buildWorkerManager(cfg.Bee, s, engines, engineCfg, envSvc)

	dispatchCh := make(chan task.DispatchTask, 128)

	sendersByPlatform := make(map[string]platform.PlatformSenderAdapter)

	// sendersByPlatform is populated below; notifier holds a reference to the same map.
	failureNotifier := task.NewPlatformFailureNotifier(s.msgStore, sendersByPlatform)
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engines, engineCfg, envSvc)

	// Local platform — always enabled, separate gateway with short debounce
	localHub := local.NewSSEHub()
	localReceiver := local.NewLocalReceiver(64)
	rawLocalSender := local.NewLocalSender(localHub)
	localSender := store.NewLoggingPlatformSenderAdapter(rawLocalSender, s.outboundMsgStore, local.PlatformID)
	sendersByPlatform[local.PlatformID] = localSender

	platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Media)
	for _, p := range platforms {
		sendersByPlatform[p.ID()] = store.NewLoggingPlatformSenderAdapter(p.Sender(), s.outboundMsgStore, p.ID())
	}

	disp := buildDispatcher(s, mgr, dispatchCh, failureNotifier, engineCfg)
	beeBusy := command.NewBeeBusyChecker(s.msgStore, s.execStore)
	workerBusy := command.NewWorkerBusyChecker(s.execStore, s.taskStore)
	engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, beeBusy, workerBusy, engineCfg)
	clearCmdHandler := command.NewClearCommandHandler(s.workerStore, s.sessionStore, s.taskStore, mgr, disp, sendersByPlatform, engineCfg)
	stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, sendersByPlatform)
	cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler)
	ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
		msgingest.WithPlatformBotNames(map[string]string{
			feishu.PlatformID:   cfg.Bee.Platforms.Feishu.BotName,
			dingtalk.PlatformID: cfg.Bee.Platforms.DingTalk.BotName,
			wecom.PlatformID:    cfg.Bee.Platforms.WeCom.BotName,
			telegram.PlatformID: cfg.Bee.Platforms.Telegram.BotName,
			weixin.PlatformID:   cfg.Bee.Platforms.Weixin.BotName,
		}))
	localIngest := msgingest.New(s.msgStore, 100*time.Millisecond, cmdChain)

	beeMCPSrv := mcp.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, s.outboundMsgStore, sendersByPlatform, mgr, disp, disp, s.execStore, s.constraintStore, s.sessionStore, s.departmentStore)

	// Synchronous startup recovery — must run before goroutines start
	feeder.RecoverFeeding(context.Background())
	sched.RecoverRunning(context.Background())

	tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, engines, ai.AllEngines())
	runners := []func(ctx context.Context){
		func(ctx context.Context) { ingest.Run(ctx) },
		func(ctx context.Context) { localIngest.Run(ctx) },
		func(ctx context.Context) {
			if err := localReceiver.Start(ctx, localIngest.Dispatch); err != nil {
				logger.Error("local receiver error", zap.Error(err))
			}
		},
		func(ctx context.Context) { feeder.Run(ctx) },
		func(ctx context.Context) { sched.Run(ctx) },
		func(ctx context.Context) { disp.Run(ctx) },
		func(ctx context.Context) { tokenSyncer.Run(ctx) },
	}
	for _, p := range platforms {
		recv := p.Receiver()
		runners = append(runners, func(ctx context.Context) {
			if err := recv.Start(ctx, ingest.Dispatch); err != nil {
				logger.Error("platform receiver error", zap.Error(err))
			}
		})
	}

	localChatHandler := api.NewLocalChatHandler(
		localReceiver, localHub,
		s.outboundMsgStore,
		s.msgStore,
	)

	srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, localChatHandler, cfg.Language, envSvc, engineCfg, disp)
	if err != nil {
		return nil, fmt.Errorf("building API server: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &App{db: db, server: srv, runners: runners, addr: addr}, nil
}

// appStores groups all store instances for passing to sub-builders.
// Named appStores (not stores) to avoid collision with the store package.
type appStores struct {
	workerStore       *store.WorkerStore
	envConfigStore    *store.EnvConfigStore
	systemConfigStore *store.SystemConfigStore
	execStore         *store.ExecutionStore
	msgStore          *store.MessageStore
	taskStore         *store.TaskStore
	sessionStore      *store.SessionStore
	outboundMsgStore  *store.OutboundMessageStore
	constraintStore   *store.ConstraintStore
	departmentStore   *store.DepartmentStore
	statsStore        *store.StatsStore
	tokenStatsStore   *store.TokenStatsStore
}

func buildStores(cfg config.DatabaseConfig) (*sql.DB, appStores, error) {
	db, err := store.InitDB(cfg.Path)
	if err != nil {
		return nil, appStores{}, fmt.Errorf("init database: %w", err)
	}
	return db, appStores{
		workerStore:       store.NewWorkerStore(db),
		envConfigStore:    store.NewEnvConfigStore(db),
		systemConfigStore: store.NewSystemConfigStore(db),
		execStore:         store.NewExecutionStore(db, config.DefaultLogsDir()),
		msgStore:          store.NewMessageStore(db),
		taskStore:         store.NewTaskStore(db),
		sessionStore:      store.NewSessionStore(db),
		outboundMsgStore:  store.NewOutboundMessageStore(db),
		constraintStore:   store.NewConstraintStore(db),
		departmentStore:   store.NewDepartmentStore(db),
		statsStore:        store.NewStatsStore(db),
		tokenStatsStore:   store.NewTokenStatsStore(db),
	}, nil
}

// buildAllEngines initializes engine adapters shared safely across concurrent workers.
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
	os.Setenv("OPENBEE_URL", cfg.MCPBaseURL) //nolint:errcheck

	result := make(map[string]ai.EngineAdapter)
	for _, name := range ai.AllEngines() {
		if !cfg.Engines.IsEnabled(name) {
			continue
		}
		adapter, err := ai.New(name, ai.EngineConfig{
			Raw: cfg.EngineConfigRawFor(name),
		})
		if err != nil {
			return nil, fmt.Errorf("init engine %q: %w", name, err)
		}
		result[name] = adapter
	}
	return result, nil
}

func buildWorkerManager(bc config.BeeConfig, s appStores, engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store, envSvc *env.Service) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore, engines, engineCfg, envSvc, s.systemConfigStore)
}

func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
	failureNotifier bee.FailureNotifier, engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store, envSvc *env.Service) (*bee.Feeder, *task.Scheduler) {
	dynamic := ai.NewDynamicAdapter(engines, engineCfg)
	beeProcess := bee.NewBeeProcess(cfg, dynamic, envSvc, s.systemConfigStore, engineCfg)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, beeProcess, config.DefaultBeeWorkDir(), cfg, engineCfg,
		bee.WithFailureNotifier(failureNotifier),
		bee.WithWorkerDispatch(s.workerStore))
	sched := task.NewScheduler(s.taskStore, dispatchCh, bee.PollInterval)
	return feeder, sched
}

func buildDispatcher(
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task.DispatchTask,
	failureNotifier task.FailureNotifier,
	engineCfg *enginecfg.Store,
) *task.TaskDispatcher {
	return task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh, engineCfg,
		task.WithFailureNotifier(failureNotifier),
		task.WithWorkerLookup(s.workerStore),
	)
}

func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, tc config.TelegramConfig, wxc config.WeixinConfig, mc config.MediaConfig) []platform.Platform {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		platform.RegisterExtractor(feishu.PlatformID, feishu.ExtractContext)
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		platform.RegisterExtractor(dingtalk.PlatformID, dingtalk.ExtractContext)
		result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc))
	}
	if wc.Enabled {
		platform.RegisterExtractor(wecom.PlatformID, wecom.ExtractContext)
		result = append(result, wecom.NewPlatform(wc, mediaSvc))
	}
	if tc.Enabled {
		result = append(result, telegram.NewPlatform(tc, mediaSvc))
	}
	if wxc.Enabled {
		result = append(result, weixin.NewPlatform(wxc, mc, mediaSvc))
	}
	return result
}

func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, localChat *api.LocalChatHandler, language string, envSvc *env.Service, engineCfg *enginecfg.Store, taskCanceller api.TaskCanceller) (*routes.Server, error) {
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	authHandler := auth.NewAuthHandler(serverCfg.Auth.Username, serverCfg.Auth.Password, jwtSvc, rateLimiter)
	jwtMiddleware := auth.JWTMiddleware(jwtSvc)
	mcpAuthMiddleware := mcp.JWTAuthMiddleware(mcpCfg.TokenSecret)

	return routes.NewServer(routes.ServerParams{
		Workers:           api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr, language),
		Executions:        api.NewExecutionHandler(s.execStore, s.tokenStatsStore),
		Messages:          api.NewMessageHandler(s.msgStore),
		Tasks:             api.NewTaskHandler(s.taskStore, s.workerStore, taskCanceller),
		Departments:       api.NewDepartmentHandler(s.departmentStore, s.workerStore),
		Stats:             api.NewStatsHandler(s.statsStore),
		Config:            api.NewConfigHandler(language, mgr.EnabledEngines()),
		LocalChat:         localChat,
		Auth:              authHandler,
		Envs:              api.NewEnvHandler(envSvc),
		SystemConfigs:     api.NewSystemConfigHandler(s.systemConfigStore, mgr, engineCfg),
		BeeMCP:            beeMCPSrv,
		MCPAuthMiddleware: mcpAuthMiddleware,
		StaticFS:          webui.DistFS,
		JWTMiddleware:     jwtMiddleware,
	})
}
