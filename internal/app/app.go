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

	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	ai "github.com/theopenbee/openbee/internal/ai"
	_ "github.com/theopenbee/openbee/internal/ai/claude"
	_ "github.com/theopenbee/openbee/internal/ai/codex"
	_ "github.com/theopenbee/openbee/internal/ai/pi"
	"github.com/theopenbee/openbee/internal/mcp"
	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/dingtalk"
	"github.com/theopenbee/openbee/internal/platform/feishu"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/platform/telegram"
	"github.com/theopenbee/openbee/internal/platform/wecom"
	"github.com/theopenbee/openbee/internal/platform/weixin"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/domain/worker"
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

	engine, err := buildEngine(cfg.Bee)
	if err != nil {
		return nil, fmt.Errorf("init engine: %w", err)
	}
	envSvc := env.NewService(s.envConfigStore, s.workerStore, s.departmentStore, cfg.Advanced.EnvSecret)
	mgr := buildWorkerManager(cfg.Bee, s, engine, envSvc)

	dispatchCh := make(chan task.DispatchTask, 128)

	sendersByPlatform := make(map[string]platform.PlatformSenderAdapter)

	// sendersByPlatform is populated below; notifier holds a reference to the same map.
	failureNotifier := task.NewPlatformFailureNotifier(s.msgStore, sendersByPlatform)
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engine, envSvc)
	ingest, disp := buildPipeline(cfg.Bee.MessageDebounce, cfg.Bee.EffectiveEngine(), s, mgr, dispatchCh, failureNotifier)

	// Local platform — always enabled, separate gateway with short debounce
	localHub := local.NewSSEHub()
	localReceiver := local.NewLocalReceiver(64)
	rawLocalSender := local.NewLocalSender(localHub)
	localSender := store.NewLoggingPlatformSenderAdapter(rawLocalSender, s.outboundMsgStore, local.PlatformID)
	localIngest := msgingest.New(s.msgStore, 100*time.Millisecond)
	sendersByPlatform[local.PlatformID] = localSender

	beeMCPSrv := mcp.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, sendersByPlatform, mgr, disp, s.execStore, s.memoryStore, s.sessionStore, s.departmentStore)
	platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Media)

	for _, p := range platforms {
		sendersByPlatform[p.ID()] = store.NewLoggingPlatformSenderAdapter(p.Sender(), s.outboundMsgStore, p.ID())
	}

	// Synchronous startup recovery — must run before goroutines start
	feeder.RecoverFeeding(context.Background())
	sched.RecoverRunning(context.Background())

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

	srv, err := buildAPIServer(cfg.Server, cfg.Bee.MCP, s, mgr, beeMCPSrv, localChatHandler, cfg.Language, envSvc)
	if err != nil {
		return nil, fmt.Errorf("building API server: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &App{db: db, server: srv, runners: runners, addr: addr}, nil
}

// appStores groups all store instances for passing to sub-builders.
// Named appStores (not stores) to avoid collision with the store package.
type appStores struct {
	workerStore      *store.WorkerStore
	envConfigStore   *store.EnvConfigStore
	execStore        *store.ExecutionStore
	msgStore         *store.MessageStore
	taskStore        *store.TaskStore
	sessionStore     *store.SessionStore
	outboundMsgStore *store.OutboundMessageStore
	memoryStore      *store.MemoryStore
	departmentStore  *store.DepartmentStore
	statsStore       *store.StatsStore
}

func buildStores(cfg config.DatabaseConfig) (*sql.DB, appStores, error) {
	db, err := store.InitDB(cfg.Path)
	if err != nil {
		return nil, appStores{}, fmt.Errorf("init database: %w", err)
	}
	return db, appStores{
		workerStore:      store.NewWorkerStore(db),
		execStore:        store.NewExecutionStore(db, config.DefaultLogsDir()),
		msgStore:         store.NewMessageStore(db),
		taskStore:        store.NewTaskStore(db),
		sessionStore:     store.NewSessionStore(db),
		outboundMsgStore: store.NewOutboundMessageStore(db),
		memoryStore:      store.NewMemoryStore(db),
		departmentStore:  store.NewDepartmentStore(db),
		statsStore:       store.NewStatsStore(db),
		envConfigStore:   store.NewEnvConfigStore(db),
	}, nil
}

func buildEngine(cfg config.BeeConfig) (ai.EngineAdapter, error) {
	return ai.New(cfg.EffectiveEngine(), ai.EngineConfig{
		OpenbeeURL: cfg.MCPBaseURL,
		Raw:        cfg.EngineConfigRaw(),
	})
}

func buildWorkerManager(bc config.BeeConfig, s appStores, engine ai.EngineAdapter, envSvc *env.Service) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore, engine, envSvc)
}

func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
	failureNotifier bee.FailureNotifier, engine ai.EngineAdapter, envSvc *env.Service) (*bee.Feeder, *task.Scheduler) {
	beeProcess := bee.NewBeeProcess(cfg, engine, envSvc)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, beeProcess, config.DefaultBeeWorkDir(), cfg,
		bee.WithFailureNotifier(failureNotifier),
		bee.WithWorkerDispatch(s.workerStore))
	sched := task.NewScheduler(s.taskStore, dispatchCh, bee.PollInterval)
	return feeder, sched
}

func buildPipeline(
	debounce time.Duration,
	engineName string,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task.DispatchTask,
	failureNotifier task.FailureNotifier,
) (*msgingest.Gateway, *task.TaskDispatcher) {
	ingest := msgingest.New(s.msgStore, debounce)
	disp := task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh,
		task.WithFailureNotifier(failureNotifier),
		task.WithEngine(engineName),
		task.WithWorkerLookup(s.workerStore),
	)
	return ingest, disp
}

func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, tc config.TelegramConfig, wxc config.WeixinConfig, mc config.MediaConfig) []platform.Platform {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc))
	}
	if wc.Enabled {
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

func buildAPIServer(serverCfg config.ServerConfig, mcpCfg config.MCPConfig, s appStores, mgr *worker.Manager, beeMCPSrv *mcp.MCPServer, localChat *api.LocalChatHandler, language string, envSvc *env.Service) (*routes.Server, error) {
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	authHandler := auth.NewAuthHandler(serverCfg.Auth.Username, serverCfg.Auth.Password, jwtSvc, rateLimiter)
	jwtMiddleware := auth.JWTMiddleware(jwtSvc)
	mcpAuthMiddleware := mcp.JWTAuthMiddleware(mcpCfg.TokenSecret)

	return routes.NewServer(routes.ServerParams{
		Workers:           api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr),
		Executions:        api.NewExecutionHandler(s.execStore),
		Tasks:             api.NewTaskHandler(s.taskStore, s.workerStore),
		Departments:       api.NewDepartmentHandler(s.departmentStore, s.workerStore),
		Stats:             api.NewStatsHandler(s.statsStore),
		Config:            api.NewConfigHandler(language),
		LocalChat:         localChat,
		Auth:              authHandler,
		Envs:              api.NewEnvHandler(envSvc),
		BeeMCP:            beeMCPSrv,
		MCPAuthMiddleware: mcpAuthMiddleware,
		StaticFS:          webui.DistFS,
		JWTMiddleware:     jwtMiddleware,
	})
}
