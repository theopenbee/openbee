package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	_ "github.com/theopenbee/openbee/internal/ai/pi"
	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/env"
	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
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
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
	"github.com/theopenbee/openbee/internal/rpc"
	"github.com/theopenbee/openbee/internal/tokenstat"
	webui "github.com/theopenbee/openbee/web"
)

// App holds all wired-up components and runs the server.
type App struct {
	db      *sql.DB
	server  *routes.Server
	runners []func(ctx context.Context)
	plugins *pluginmgr.Manager
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
	if err := a.plugins.Wait(shutdownCtx); err != nil {
		logger.Warn("timed out waiting for plugin channels to stop")
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

	sendersByPlatform := platform.NewSenderRegistry()

	failureNotifier := task.NewPlatformFailureNotifier(s.msgStore, sendersByPlatform)
	localMediaRoot, err := config.LocalUploadsDir()
	if err != nil {
		logger.Warn("local chat media disabled: cannot resolve uploads dir", zap.Error(err))
	}
	feeder, sched := buildBee(cfg.Bee, s, dispatchCh, failureNotifier, engines, engineCfg, envSvc, localMediaRoot)

	// Local platform — always enabled, separate gateway with short debounce
	localHub := local.NewSSEHub()
	localPub := local.NewEventPublisher(localHub)
	localReceiver := local.NewLocalReceiver(64)
	legacyChat := local.NewLegacySession(s.sessionStore, localMediaRoot)
	rawLocalSender := local.NewLocalSender(localPub, localMediaRoot)
	localSender := store.NewLoggingPlatformSenderAdapter(rawLocalSender, s.outboundMsgStore, local.PlatformID)
	sendersByPlatform.Set(local.PlatformID, localSender)

	if len(cfg.Bee.LegacyPlatforms) > 0 {
		logger.Warn("bee.platforms is no longer used; manage plugins with openbee plugin or the web UI")
	}
	pluginMgr, err := buildPluginManager(cfg, s, sendersByPlatform)
	if err != nil {
		return nil, err
	}
	mgr.SetBotNameSource(pluginMgr.BotNameList)

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
	ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain)
	localIngest := msgingest.New(s.msgStore, 100*time.Millisecond, cmdChain)

	beeRPCSrv := rpc.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, s.outboundMsgStore, sendersByPlatform, clearSvc, s.execStore, s.constraintStore, s.sessionStore, s.departmentStore)

	if err := legacyChat.Restore(context.Background(), s.userStore.AdminID); err != nil {
		logger.Error("restore legacy local chat", zap.Error(err))
	}

	// Synchronous startup recovery — must run before goroutines start
	feeder.RecoverFeeding(context.Background())
	sched.RecoverRunning(context.Background())
	if n, err := s.execStore.ResetRunningExecutions(context.Background()); err != nil {
		logger.Error("recover running executions", zap.Error(err))
	} else if n > 0 {
		logger.Info("reset orphaned executions", zap.Int64("count", n))
	}

	tokenSyncer := tokenstat.NewSyncer(db, s.tokenStatsStore, engines, ai.AllEngines())
	reconciler := task.NewReconciler(s.taskStore, s.execStore, 0)
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
		func(ctx context.Context) { reconciler.Run(ctx) },
		func(ctx context.Context) { tokenSyncer.Run(ctx) },
	}
	runners = append(runners, func(ctx context.Context) {
		pluginMgr.Start(ctx, ingest.Dispatch, ingest.SetPlatformBotNames)
	})

	localChatHandler := api.NewLocalChatHandler(
		localReceiver, localHub,
		s.outboundMsgStore,
		s.msgStore,
		localMediaRoot,
	)

	srv, err := buildAPIServer(cfg.Server, cfg.Bee.RPC, s, mgr, beeRPCSrv, localChatHandler, legacyChat.Claim, cfg.Language, envSvc, engineCfg, pluginMgr)
	if err != nil {
		return nil, fmt.Errorf("building API server: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &App{db: db, server: srv, runners: runners, plugins: pluginMgr, addr: addr}, nil
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
	tokenStatsStore   *store.TokenStatsStore
	userStore         *store.UserStore
	pluginSettings    *store.PluginSettingsStore
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
		tokenStatsStore:   store.NewTokenStatsStore(db),
		userStore:         store.NewUserStore(db),
		pluginSettings:    store.NewPluginSettingsStore(db),
	}, nil
}

// buildAllEngines initializes engine adapters shared safely across concurrent workers.
func buildAllEngines(cfg config.BeeConfig) (map[string]ai.EngineAdapter, error) {
	os.Setenv("OPENBEE_URL", cfg.RPCBaseURL) //nolint:errcheck

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
	failureNotifier bee.FailureNotifier, engines map[string]ai.EngineAdapter, engineCfg *enginecfg.Store, envSvc *env.Service, localMediaRoot string) (*bee.Feeder, *task.Scheduler) {
	dynamic := ai.NewDynamicAdapter(engines, engineCfg)
	beeProcess := bee.NewBeeProcess(cfg, dynamic, envSvc, s.systemConfigStore, engineCfg)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, s.execStore, beeProcess, config.DefaultBeeWorkDir(), cfg, engineCfg,
		bee.WithFailureNotifier(failureNotifier),
		bee.WithWorkerDispatch(s.workerStore),
		bee.WithLocalMediaRoot(localMediaRoot))
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

func buildAPIServer(serverCfg config.ServerConfig, rpcCfg config.RPCConfig, s appStores, mgr *worker.Manager, beeRPCSrv *rpc.Server, localChat *api.LocalChatHandler, claimLegacyChat func(ctx context.Context, userID string) error, language string, envSvc *env.Service, engineCfg *enginecfg.Store, plugins *pluginmgr.Manager) (*routes.Server, error) {
	secret := serverCfg.Auth.JWTSecret
	jwtSvc := auth.NewJWTService(secret, serverCfg.Auth.AccessTokenTTL, serverCfg.Auth.RefreshTokenTTL)
	rateLimiter := auth.NewLoginRateLimiter(5, time.Minute)
	authHandler := auth.NewAuthHandler(s.userStore, jwtSvc, rateLimiter)
	authMiddleware := auth.AuthMiddleware(jwtSvc, s.userStore)
	rpcAuthMiddleware := rpc.JWTAuthMiddleware(rpcCfg.TokenSecret)

	return routes.NewServer(routes.ServerParams{
		Workers:           api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr, language),
		Departments:       api.NewDepartmentHandler(s.departmentStore, s.workerStore),
		Config:            api.NewConfigHandler(language, mgr.EnabledEngines()),
		LocalChat:         localChat,
		Auth:              authHandler,
		Envs:              api.NewEnvHandler(envSvc),
		SystemConfigs:     api.NewSystemConfigHandler(s.systemConfigStore, mgr, engineCfg),
		Setup:             api.NewSetupHandler(s.userStore, jwtSvc, claimLegacyChat),
		Plugins:           api.NewPluginHandler(plugins),
		BeeRPC:            beeRPCSrv,
		RPCAuthMiddleware: rpcAuthMiddleware,
		StaticFS:          webui.DistFS,
		AuthMiddleware:    authMiddleware,
		PluginsAuth:       auth.PluginsAuth(jwtSvc, s.userStore, rpcCfg.TokenSecret),
	})
}

func buildPluginManager(cfg config.Config, s appStores, senders *platform.SenderRegistry) (*pluginmgr.Manager, error) {
	codec, err := pluginmgr.NewCodec(cfg.Server.EnvSecret, !cfg.Server.EnvSecretGenerated)
	if err != nil {
		return nil, fmt.Errorf("init plugin settings codec: %w", err)
	}
	runtime := plugin.NewRuntimeResolver()
	runtime.OnInstall = func(version, dir string) {
		logger.Info("no usable bun or node found, installing bun for plugins", zap.String("version", version), zap.String("dir", dir))
	}
	registry := plugin.OpenRegistry(plugin.DefaultDir())
	return pluginmgr.New(pluginmgr.Deps{
		Registry:  registry,
		Installer: &plugin.Installer{Registry: registry, Runtime: runtime, Run: plugin.ExecRunner},
		Store:     s.pluginSettings,
		Codec:     codec,
		Senders:   senders,
		WrapSender: func(id string, sender platform.PlatformSenderAdapter) platform.PlatformSenderAdapter {
			return store.NewLoggingPlatformSenderAdapter(sender, s.outboundMsgStore, id)
		},
		Factory: pluginmgr.PluginChannelFactory(runtime, media.NewService(), s.taskStore, cfg.Language),
		WorkerNameTaken: func(name string) (bool, error) {
			return s.workerStore.ExistsByName(name, "")
		},
		Catalog: catalog.NewClient(catalog.Options{
			CachePath: filepath.Join(plugin.DefaultDir(), "catalog.json"),
			Override:  os.Getenv("OPENBEE_PLUGIN_CATALOG"),
		}),
		Version: buildinfo.Version,
		SetupCommand: func(ctx context.Context, man plugin.Manifest, lang string) (*exec.Cmd, error) {
			return plugin.SetupCommand(ctx, runtime, man, lang)
		},
		Language: cfg.Language,
	}), nil
}
