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
	"golang.org/x/sync/errgroup"

	ai "github.com/theopenbee/openbee/internal/ai"
	_ "github.com/theopenbee/openbee/internal/ai/claude"
	_ "github.com/theopenbee/openbee/internal/ai/codex"
	_ "github.com/theopenbee/openbee/internal/ai/pi"
	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/buildinfo"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/dingtalk"
	"github.com/theopenbee/openbee/internal/platform/feishu"
	"github.com/theopenbee/openbee/internal/platform/linear"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/platform/telegram"
	"github.com/theopenbee/openbee/internal/platform/wecom"
	"github.com/theopenbee/openbee/internal/platform/weixin"
	"github.com/theopenbee/openbee/internal/rpc"
	"github.com/theopenbee/openbee/internal/tokenstat"
	webui "github.com/theopenbee/openbee/web"
)

// shutdownTimeout bounds how long the HTTP server gets to finish in-flight
// requests once shutdown begins.
const shutdownTimeout = 15 * time.Second

// runner is a named long-lived background loop. run must return when ctx is
// cancelled; Run waits for every one of them before closing the database.
type runner struct {
	name string
	run  func(ctx context.Context)
}

// httpServer is the slice of *routes.Server that App depends on. Keeping it an
// interface lets the lifecycle be tested without standing up the full router.
type httpServer interface {
	Run(addr string) error
	Shutdown(ctx context.Context) error
}

// App holds all wired-up components and runs the server.
type App struct {
	db      *sql.DB
	server  httpServer
	runners []runner
	addr    string

	// recoverInflight re-hydrates work left behind by a previous process. It
	// runs at the start of Run rather than during BuildApp, so constructing an
	// App has no side effects.
	recoverInflight func(ctx context.Context)
}

// Run starts all background runners and the HTTP server, then blocks until
// SIGINT/SIGTERM or an unrecoverable server error.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return a.run(ctx)
}

// run is Run without signal handling, so tests can drive shutdown directly.
func (a *App) run(ctx context.Context) error {
	// Deferred first, so it unwinds last — after g.Wait() below has confirmed
	// every runner returned. Runners touch the database on their way out of a
	// cancelled loop, so closing it any earlier is a race.
	defer a.db.Close()

	a.recoverInflight(ctx)

	g, gctx := errgroup.WithContext(ctx)

	for _, r := range a.runners {
		g.Go(func() error {
			r.run(gctx)
			logger.Debug("runner exited", zap.String("runner", r.name))
			return nil
		})
	}

	g.Go(func() error {
		logger.Info("OpenBee Core starting", zap.String("addr", a.addr))
		// A non-nil return here cancels gctx, which drains every runner.
		if err := a.server.Run(a.addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-gctx.Done()
		logger.Info("Shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown: %w", err)
		}
		return nil
	})

	err := g.Wait()
	logger.Info("all runners drained")
	return err
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

	if err := publishRPCBaseURL(cfg.Bee); err != nil {
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

	// Local platform — always enabled, separate gateway with short debounce
	localHub := local.NewSSEHub()
	localReceiver := local.NewLocalReceiver(64)
	rawLocalSender := local.NewLocalSender(localHub)
	localSender := store.NewLoggingPlatformSenderAdapter(rawLocalSender, s.outboundMsgStore, local.PlatformID)

	platforms, err := buildPlatforms(
		cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom,
		cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Platforms.Linear,
		cfg.Bee.Media,
	)
	if err != nil {
		return nil, err
	}

	// Built in one shot, before any consumer sees it. Nothing may capture a
	// reference to this map while it is still being filled.
	sendersByPlatform := buildSenders(platforms, localSender, s.outboundMsgStore)

	failureNotifier := task.NewPlatformFailureNotifier(s.msgStore, sendersByPlatform)
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engines, engineCfg, envSvc)

	disp := buildDispatcher(s, mgr, dispatchCh, failureNotifier, engineCfg)
	beeBusy := command.NewBeeBusyChecker(s.msgStore, s.execStore)
	workerBusy := command.NewWorkerBusyChecker(s.execStore, s.taskStore)
	engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, beeBusy, workerBusy, engineCfg)
	clearSvc := session.NewClearService(session.ClearServiceDeps{
		Sessions:      s.sessionStore,
		Tasks:         s.taskStore,
		ExecStopper:   mgr,
		ExecFinalizer: s.execStore,
		Dispatcher:    disp,
		TaskCanceller: disp,
		RunningExecs:  s.execStore,
		EngineCfg:     engineCfg,
	})
	clearCmdHandler := command.NewClearCommandHandler(s.workerStore, clearSvc, sendersByPlatform, s.execStore)
	stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, s.workerStore, clearSvc, sendersByPlatform)
	statusCmdHandler := command.NewStatusCommandHandler(command.StatusCommandDeps{
		Sessions:     s.sessionStore,
		Tasks:        s.taskStore,
		Workers:      s.workerStore,
		Senders:      sendersByPlatform,
		EngineCfg:    engineCfg,
		RunningExecs: s.execStore,
	})
	listCmdHandler := command.NewListCommandHandler(s.workerStore, sendersByPlatform)
	cmdChain := msgingest.ChainHandlers(engineCmdHandler, clearCmdHandler, stopCmdHandler, statusCmdHandler, listCmdHandler)
	ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
		msgingest.WithPlatformBotNames(map[string]string{
			feishu.PlatformID:   cfg.Bee.Platforms.Feishu.BotName,
			dingtalk.PlatformID: cfg.Bee.Platforms.DingTalk.BotName,
			wecom.PlatformID:    cfg.Bee.Platforms.WeCom.BotName,
			telegram.PlatformID: cfg.Bee.Platforms.Telegram.BotName,
			weixin.PlatformID:   cfg.Bee.Platforms.Weixin.BotName,
		}))
	localIngest := msgingest.New(s.msgStore, 100*time.Millisecond, cmdChain)

	beeRPCSrv := rpc.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, s.outboundMsgStore, sendersByPlatform, clearSvc, s.execStore, s.constraintStore, s.sessionStore, s.departmentStore)

	// Startup recovery is deferred to Run so that building an App stays free of
	// side effects. It still completes before any runner starts.
	recoverInflight := func(ctx context.Context) {
		feeder.RecoverFeeding(ctx)
		sched.RecoverRunning(ctx)
		if n, err := s.execStore.ResetRunningExecutions(ctx); err != nil {
			logger.Error("recover running executions", zap.Error(err))
		} else if n > 0 {
			logger.Info("reset orphaned executions", zap.Int64("count", n))
		}
	}

	tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, engines, ai.AllEngines())
	reconciler := task.NewReconciler(s.taskStore, s.execStore, 0)
	runners := []runner{
		{name: "ingest", run: ingest.Run},
		{name: "ingest:local", run: localIngest.Run},
		{name: "receiver:local", run: func(ctx context.Context) {
			if err := localReceiver.Start(ctx, localIngest.Dispatch); err != nil {
				logger.Error("local receiver error", zap.Error(err))
			}
		}},
		{name: "feeder", run: feeder.Run},
		{name: "scheduler", run: sched.Run},
		{name: "dispatcher", run: disp.Run},
		{name: "reconciler", run: reconciler.Run},
		{name: "tokenstat", run: tokenSyncer.Run},
	}
	for _, p := range platforms {
		recv, id := p.Receiver(), p.ID()
		runners = append(runners, runner{
			name: "receiver:" + id,
			run: func(ctx context.Context) {
				if err := recv.Start(ctx, ingest.Dispatch); err != nil {
					logger.Error("platform receiver error", zap.String("platform", id), zap.Error(err))
				}
			},
		})
	}

	localChatHandler := api.NewLocalChatHandler(
		localReceiver, localHub,
		s.outboundMsgStore,
		s.msgStore,
	)

	srv, err := buildAPIServer(cfg.Server, cfg.Bee.RPC, s, mgr, beeRPCSrv, localChatHandler, cfg.Language, envSvc, engineCfg, disp)
	if err != nil {
		return nil, fmt.Errorf("building API server: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &App{
		db:              db,
		server:          srv,
		runners:         runners,
		addr:            addr,
		recoverInflight: recoverInflight,
	}, nil
}

// buildSenders returns the complete platform-id -> sender map.
func buildSenders(
	platforms []platform.Platform,
	localSender platform.PlatformSenderAdapter,
	out *store.OutboundMessageStore,
) map[string]platform.PlatformSenderAdapter {
	senders := map[string]platform.PlatformSenderAdapter{
		local.PlatformID: localSender,
	}
	for _, p := range platforms {
		senders[p.ID()] = store.NewLoggingPlatformSenderAdapter(p.Sender(), out, p.ID())
	}
	return senders
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
	userStore         *store.UserStore
	roleStore         *store.RoleStore
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
		userStore:         store.NewUserStore(db),
		roleStore:         store.NewRoleStore(db),
	}, nil
}

// publishRPCBaseURL exports the RPC base URL to the process environment.
// Engine CLIs (claude/codex/pi) read OPENBEE_URL from their inherited env, so
// this must run before any engine adapter is constructed.
func publishRPCBaseURL(cfg config.BeeConfig) error {
	if err := os.Setenv("OPENBEE_URL", cfg.RPCBaseURL); err != nil {
		return fmt.Errorf("publish OPENBEE_URL: %w", err)
	}
	return nil
}

// buildAllEngines initializes engine adapters shared safely across concurrent workers.
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
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

func buildPlatforms(
	fc config.FeishuConfig,
	dc config.DingTalkConfig,
	wc config.WeComConfig,
	tc config.TelegramConfig,
	wxc config.WeixinConfig,
	lc config.LinearConfig,
	mc config.MediaConfig,
) ([]platform.Platform, error) {
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
	if lc.Enabled {
		p, err := linear.NewPlatform(lc, mediaSvc)
		if err != nil {
			return nil, fmt.Errorf("init linear platform: %w", err)
		}
		result = append(result, p)
	}
	return result, nil
}

func buildAPIServer(serverCfg config.ServerConfig, rpcCfg config.RPCConfig, s appStores, mgr *worker.Manager, beeRPCSrv *rpc.Server, localChat *api.LocalChatHandler, language string, envSvc *env.Service, engineCfg *enginecfg.Store, taskCanceller api.TaskCanceller) (*routes.Server, error) {
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	resolver := auth.NewPermissionResolver(s.userStore.PermissionsForUser)
	authHandler := auth.NewAuthHandler(s.userStore, jwtSvc, rateLimiter, resolver)
	authMiddleware := auth.AuthMiddleware(jwtSvc, s.userStore)
	rpcAuthMiddleware := rpc.JWTAuthMiddleware(rpcCfg.TokenSecret)

	return routes.NewServer(routes.ServerParams{
		Workers:           api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr, language),
		Executions:        api.NewExecutionHandler(s.execStore, s.tokenStatsStore),
		Tasks:             api.NewTaskHandler(s.taskStore, s.workerStore, taskCanceller),
		Departments:       api.NewDepartmentHandler(s.departmentStore, s.workerStore),
		Stats:             api.NewStatsHandler(s.statsStore),
		Config:            api.NewConfigHandler(language, mgr.EnabledEngines()),
		Version:           api.NewVersionHandler(buildinfo.Get()),
		LocalChat:         localChat,
		Auth:              authHandler,
		Envs:              api.NewEnvHandler(envSvc),
		SystemConfigs:     api.NewSystemConfigHandler(s.systemConfigStore, mgr, engineCfg),
		Users:             api.NewUserHandler(s.userStore, resolver),
		Roles:             api.NewRoleHandler(s.roleStore, resolver),
		Setup:             api.NewSetupHandler(s.userStore, jwtSvc),
		BeeRPC:            beeRPCSrv,
		RPCAuthMiddleware: rpcAuthMiddleware,
		StaticFS:          webui.DistFS,
		AuthMiddleware:    authMiddleware,
		Resolver:          resolver,
	})
}
