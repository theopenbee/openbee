package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robobee/core/internal/api"
	"github.com/robobee/core/internal/bee"
	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/task_dispatcher"
	"github.com/robobee/core/internal/mcp"
	"github.com/robobee/core/internal/msgingest"
	"github.com/robobee/core/internal/platform"
	"github.com/robobee/core/internal/media"
	"github.com/robobee/core/internal/platform/dingtalk"
	"github.com/robobee/core/internal/platform/feishu"
	"github.com/robobee/core/internal/store"
	"github.com/robobee/core/internal/task_scheduler"
	"github.com/robobee/core/internal/worker"
	webui "github.com/robobee/core/web"
)

// App holds all wired-up components and runs the server.
type App struct {
	db      *sql.DB
	server  *api.Server
	runners []func(ctx context.Context)
	addr    string
}

// Run starts all goroutines, waits for a signal, then shuts down.
func (a *App) Run() {
	ctx, cancel := context.WithCancel(context.Background())

	for _, r := range a.runners {
		r := r
		go r(ctx)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("Shutting down...")
		cancel()
		a.db.Close()
		os.Exit(0)
	}()

	slog.Info("RoboBee Core starting", "addr", a.addr)
	if err := a.server.Run(a.addr); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// buildApp wires all components together. Returns a ready-to-run App.
func buildApp(cfg config.Config) (*App, error) {
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	if cfg.Bee.MCP.APIKey == "" {
		return nil, fmt.Errorf("bee.mcp.api_key must be set — bee requires MCP to create tasks")
	}

	db, s, err := buildStores(cfg.Database)
	if err != nil {
		return nil, err
	}

	mgr := buildWorkerManager(cfg.Bee, s)

	dispatchCh := make(chan task_dispatcher.DispatchTask, 128)

	sendersByPlatform := make(map[string]platform.PlatformSenderAdapter)

	feeder, sched := buildBee(cfg.Bee, s, dispatchCh)
	ingest, disp := buildPipeline(cfg.Bee.MessageDebounce, s, mgr, dispatchCh)

	mcpSrv := mcp.NewServer(s.workerStore, mgr, s.taskStore, s.msgStore, sendersByPlatform, mgr, disp)
	platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk)

	// Populate sender map before goroutines start
	for _, p := range platforms {
		sendersByPlatform[p.ID()] = p.Sender()
	}

	// Synchronous startup recovery — must run before goroutines start
	feeder.RecoverFeeding(context.Background())
	sched.RecoverRunning(context.Background())

	runners := []func(ctx context.Context){
		func(ctx context.Context) { ingest.Run(ctx) },
		func(ctx context.Context) { feeder.Run(ctx) },
		func(ctx context.Context) { sched.Run(ctx) },
		func(ctx context.Context) { disp.Run(ctx) },
	}
	for _, p := range platforms {
		recv := p.Receiver()
		runners = append(runners, func(ctx context.Context) {
			if err := recv.Start(ctx, ingest.Dispatch); err != nil {
				slog.Error("platform receiver error", "error", err)
			}
		})
	}

	srv := buildAPIServer(cfg.Bee.MCP, s, mgr, mcpSrv)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &App{db: db, server: srv, runners: runners, addr: addr}, nil
}

// appStores groups all store instances for passing to sub-builders.
// Named appStores (not stores) to avoid collision with the store package.
type appStores struct {
	workerStore  *store.WorkerStore
	execStore    *store.ExecutionStore
	msgStore     *store.MessageStore
	taskStore    *store.TaskStore
	sessionStore *store.SessionStore
}

func buildStores(cfg config.DatabaseConfig) (*sql.DB, appStores, error) {
	db, err := store.InitDB(cfg.Path)
	if err != nil {
		return nil, appStores{}, fmt.Errorf("init database: %w", err)
	}
	return db, appStores{
		workerStore:  store.NewWorkerStore(db),
		execStore:    store.NewExecutionStore(db),
		msgStore:     store.NewMessageStore(db),
		taskStore:    store.NewTaskStore(db),
		sessionStore: store.NewSessionStore(db),
	}, nil
}

func buildWorkerManager(bc config.BeeConfig, s appStores) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), bc, s.workerStore, s.execStore)
}

func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan task_dispatcher.DispatchTask) (*bee.Feeder, *task_scheduler.Scheduler) {
	beeProcess := bee.NewBeeProcess(cfg)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, beeProcess, config.DefaultBeeWorkDir(), cfg)
	sched := task_scheduler.New(s.taskStore, dispatchCh, cfg.Feeder.Interval)
	return feeder, sched
}

func buildPipeline(
	debounce time.Duration,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan task_dispatcher.DispatchTask,
) (*msgingest.Gateway, *task_dispatcher.TaskDispatcher) {
	ingest := msgingest.New(s.msgStore, debounce)
	disp := task_dispatcher.New(mgr, s.taskStore, s.sessionStore, s.execStore, dispatchCh)
	return ingest, disp
}

func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig) []platform.Platform {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		result = append(result, dingtalk.NewPlatform(dc, mediaSvc))
	}
	return result
}

func buildAPIServer(cfg config.MCPConfig, s appStores, mgr *worker.Manager, mcpSrv *mcp.MCPServer) *api.Server {
	return api.NewServer(s.workerStore, s.execStore, mgr, mcpSrv, cfg.APIKey, webui.DistFS)
}
