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

	"github.com/theopenbee/openbee/internal/bridge"
	bridgeengines "github.com/theopenbee/openbee/internal/bridge/engines"
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

	defaultEngine := cfg.Bee.EffectiveEngine()
	// Initialize the default engine store from DB, falling back to config.
	engineCfg := enginecfg.NewStore(defaultEngine)

	envSvc, err := env.NewService(s.envConfigStore, s.departmentStore, cfg.Server.EnvSecret)
	if err != nil {
		return nil, fmt.Errorf("init env service: %w", err)
	}
	aiBridge, err := buildBridge(cfg.Bee, engineCfg, envSvc, s.systemConfigStore)
	if err != nil {
		return nil, fmt.Errorf("init ai bridge: %w", err)
	}
	if err := aiBridge.ValidateEngine(defaultEngine); err != nil {
		return nil, fmt.Errorf("default engine %q is not enabled; enable it under bee.engines in config", defaultEngine)
	}

	dbCfg, found, dbErr := s.systemConfigStore.Get(context.Background(), model.SystemConfigKeyDefaultEngine)
	if dbErr != nil {
		logger.Warn("failed to load default engine from DB, falling back to config", zap.Error(dbErr))
	} else if found {
		if err := aiBridge.ValidateEngine(dbCfg.Value); err == nil {
			engineCfg.Set(dbCfg.Value)
		} else {
			logger.Warn("DB default engine is not enabled, falling back to config",
				zap.String("db_value", dbCfg.Value))
		}
	}

	mgr := buildWorkerManager(cfg.Bee, s, aiBridge)

	dispatchCh := make(chan task.DispatchTask, 128)

	sendersByPlatform := make(map[string]platform.PlatformSenderAdapter)

	// sendersByPlatform is populated below; notifier holds a reference to the same map.
	failureNotifier := task.NewPlatformFailureNotifier(s.msgStore, sendersByPlatform)
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, aiBridge, engineCfg)

	// Local platform — always enabled, separate gateway with short debounce
	localHub := local.NewSSEHub()
	localReceiver := local.NewLocalReceiver(64)
	rawLocalSender := local.NewLocalSender(localHub)
	localSender := store.NewLoggingPlatformSenderAdapter(rawLocalSender, s.outboundMsgStore, local.PlatformID)
	sendersByPlatform[local.PlatformID] = localSender

	platforms, err := buildPlatforms(
		cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom,
		cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Platforms.Linear,
		cfg.Bee.Media,
	)
	if err != nil {
		return nil, err
	}
	for _, p := range platforms {
		sendersByPlatform[p.ID()] = store.NewLoggingPlatformSenderAdapter(p.Sender(), s.outboundMsgStore, p.ID())
	}

	disp := buildDispatcher(s, mgr, dispatchCh, failureNotifier, engineCfg, aiBridge)
	beeBusy := command.NewBeeBusyChecker(s.msgStore, s.execStore)
	workerBusy := command.NewWorkerBusyChecker(s.execStore, s.taskStore)
	engineCmdHandler := command.NewEngineCommandHandler(s.workerStore, s.systemConfigStore, sendersByPlatform, mgr, beeBusy, workerBusy, engineCfg)
	clearCmdHandler := command.NewClearCommandHandler(s.workerStore, s.sessionStore, s.taskStore, mgr, disp, sendersByPlatform, engineCfg)
	stopCmdHandler := command.NewStopCommandHandler(feeder, s.msgStore, sendersByPlatform)
	statusCmdHandler := command.NewStatusCommandHandler(s.sessionStore, s.taskStore, s.workerStore, sendersByPlatform, engineCfg)
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

	beeRPCSrv := rpc.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, s.outboundMsgStore, sendersByPlatform, mgr, disp, disp, s.execStore, s.constraintStore, s.sessionStore, s.departmentStore)

	// Synchronous startup recovery — must run before goroutines start
	feeder.RecoverFeeding(context.Background())
	sched.RecoverRunning(context.Background())
	if n, err := s.execStore.ResetRunningExecutions(context.Background()); err != nil {
		logger.Error("recover running executions", zap.Error(err))
	} else if n > 0 {
		logger.Info("reset orphaned executions", zap.Int64("count", n))
	}

	tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, aiBridge)
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

	srv, err := buildAPIServer(cfg.Server, cfg.Bee.RPC, s, mgr, beeRPCSrv, localChatHandler, cfg.Language, envSvc, engineCfg, disp, aiBridge)
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

func buildBridge(bc config.BeeConfig, engineCfg *enginecfg.Store, envSvc *env.Service, sysStore *store.SystemConfigStore) (bridge.Bridge, error) {
	engines, err := bridgeengines.BuildEngines(bc.RPCBaseURL, bc.Engines.IsEnabled, bc.EngineConfigRawFor)
	if err != nil {
		return nil, err
	}
	return bridge.NewService(bridge.ServiceOptions{
		Engines:      engines,
		EngineCfg:    engineCfg,
		TokenSecret:  bc.RPC.TokenSecret,
		TokenTTL:     bc.RPC.TokenTTL,
		Env:          envSvc,
		SystemConfig: sysStore,
	}), nil
}

func buildWorkerManager(bc config.BeeConfig, s appStores, br bridge.Bridge) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore, br)
}

func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task.DispatchTask,
	failureNotifier bee.FailureNotifier, br bridge.Bridge, engineCfg *enginecfg.Store) (*bee.Feeder, *task.Scheduler) {
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, br, config.DefaultBeeWorkDir(), cfg, engineCfg,
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
	br bridge.Bridge,
) *task.TaskDispatcher {
	return task.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh, engineCfg, br,
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

func buildAPIServer(serverCfg config.ServerConfig, rpcCfg config.RPCConfig, s appStores, mgr *worker.Manager, beeRPCSrv *rpc.Server, localChat *api.LocalChatHandler, language string, envSvc *env.Service, engineCfg *enginecfg.Store, taskCanceller api.TaskCanceller, br bridge.Bridge) (*routes.Server, error) {
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	authHandler := auth.NewAuthHandler(serverCfg.Auth.Username, serverCfg.Auth.Password, jwtSvc, rateLimiter)
	jwtMiddleware := auth.JWTMiddleware(jwtSvc)
	rpcAuthMiddleware := rpc.JWTAuthMiddleware(rpcCfg.TokenSecret)

	return routes.NewServer(routes.ServerParams{
		Workers:           api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr, language),
		Executions:        api.NewExecutionHandler(s.execStore, s.tokenStatsStore),
		Messages:          api.NewMessageHandler(s.msgStore),
		Tasks:             api.NewTaskHandler(s.taskStore, s.workerStore, taskCanceller),
		Departments:       api.NewDepartmentHandler(s.departmentStore, s.workerStore),
		Stats:             api.NewStatsHandler(s.statsStore),
		Config:            api.NewConfigHandler(language, br.EnabledEngines()),
		LocalChat:         localChat,
		Auth:              authHandler,
		Envs:              api.NewEnvHandler(envSvc),
		SystemConfigs:     api.NewSystemConfigHandler(s.systemConfigStore, mgr, engineCfg),
		BeeRPC:            beeRPCSrv,
		RPCAuthMiddleware: rpcAuthMiddleware,
		StaticFS:          webui.DistFS,
		JWTMiddleware:     jwtMiddleware,
	})
}
