package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robobee/core/internal/api"
	"github.com/robobee/core/internal/bee"
	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/dispatcher"
	"github.com/robobee/core/internal/mcp"
	"github.com/robobee/core/internal/msgingest"
	"github.com/robobee/core/internal/platform"
	"github.com/robobee/core/internal/platform/dingtalk"
	"github.com/robobee/core/internal/platform/feishu"
	"github.com/robobee/core/internal/store"
	"github.com/robobee/core/internal/taskscheduler"
	"github.com/robobee/core/internal/worker"
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
		log.Println("Shutting down...")
		cancel()
		a.db.Close()
		os.Exit(0)
	}()

	log.Printf("RoboBee Core starting on %s", a.addr)
	if err := a.server.Run(a.addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// buildApp wires all components together. Returns a ready-to-run App.
func buildApp(cfg config.Config) (*App, error) {
	if cfg.MCP.APIKey == "" {
		return nil, fmt.Errorf("mcp.api_key must be set — bee requires MCP to create tasks")
	}

	db, s, err := buildStores(cfg.Database)
	if err != nil {
		return nil, err
	}

	mgr := buildWorkerManager(cfg.Runtime, s)

	dispatchCh := make(chan dispatcher.DispatchTask, 128)

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
				log.Printf("platform receiver error: %v", err)
			}
		})
	}

	srv := buildAPIServer(cfg.MCP, s, mgr, mcpSrv)
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

func buildWorkerManager(rc config.RuntimeConfig, s appStores) *worker.Manager {
	return worker.NewManager(config.DefaultWorkerBaseDir(), rc, s.workerStore, s.execStore)
}

func buildBee(cfg config.BeeConfig, s appStores, dispatchCh chan dispatcher.DispatchTask) (*bee.Feeder, *taskscheduler.Scheduler) {
	beeProcess := bee.NewBeeProcess(cfg)
	feeder := bee.NewFeeder(s.msgStore, s.taskStore, s.sessionStore, beeProcess, config.DefaultBeeWorkDir(), cfg)
	sched := taskscheduler.New(s.taskStore, dispatchCh, cfg.Feeder.Interval)
	return feeder, sched
}

func buildPipeline(
	debounce time.Duration,
	s appStores,
	mgr *worker.Manager,
	dispatchCh chan dispatcher.DispatchTask,
) (*msgingest.Gateway, *dispatcher.Dispatcher) {
	ingest := msgingest.New(s.msgStore, debounce)
	disp := dispatcher.New(mgr, s.taskStore, s.sessionStore, dispatchCh)
	return ingest, disp
}

func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig) []platform.Platform {
	var result []platform.Platform
	if fc.Enabled {
		result = append(result, feishu.NewPlatform(fc))
	}
	if dc.Enabled {
		result = append(result, dingtalk.NewPlatform(dc))
	}
	return result
}

func buildAPIServer(cfg config.MCPConfig, s appStores, mgr *worker.Manager, mcpSrv *mcp.MCPServer) *api.Server {
	return api.NewServer(s.workerStore, s.execStore, mgr, mcpSrv, cfg.APIKey)
}
