package api

import (
	"context"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/mcp"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/worker"
)

type Server struct {
	router           *gin.Engine
	httpServer       *http.Server
	workerStore      *store.WorkerStore
	executionStore   *store.ExecutionStore
	manager          *worker.Manager
	mcpServer        *mcp.MCPServer
	mcpAPIKey        string
	staticFS         fs.FS
	localChatHandler *LocalChatHandler
}

func NewServer(
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	mgr *worker.Manager,
	mcpSrv *mcp.MCPServer,
	mcpAPIKey string,
	staticFS fs.FS,
	localChat *LocalChatHandler,
) *Server {
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathsRegexs([]string{
		"/api/local/sessions/.+/stream",
		"/mcp/sse",
		"/mcp/messages",
	})))
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept-Language", "X-API-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
	}))
	s := &Server{
		router:           router,
		workerStore:      ws,
		executionStore:   es,
		manager:          mgr,
		mcpServer:        mcpSrv,
		mcpAPIKey:        mcpAPIKey,
		staticFS:         staticFS,
		localChatHandler: localChat,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	api := s.router.Group("/api")
	{
		// Workers
		api.POST("/workers", s.createWorker)
		api.GET("/workers", s.listWorkers)
		api.GET("/workers/:id", s.getWorker)
		api.PUT("/workers/:id", s.updateWorker)
		api.DELETE("/workers/:id", s.deleteWorker)

		// Worker executions
		api.GET("/workers/:id/executions", s.listWorkerExecutions)

		// Sessions
		api.GET("/sessions/:sessionId/executions", s.listSessionExecutions)

		// Executions
		api.GET("/executions", s.listExecutions)
		api.GET("/executions/:id", s.getExecution)
		// WebSocket logs
		api.GET("/executions/:id/logs", s.streamLogs)

		// Local chat
		if s.localChatHandler != nil {
			s.localChatHandler.RegisterRoutes(api)
		}
	}

	// SSE stream for local chat — registered outside gzip middleware
	if s.localChatHandler != nil {
		s.router.GET("/api/local/sessions/:id/stream", s.localChatHandler.StreamReplies)
	}

	// Internal log level control — PUT /internal/log/level with JSON body {"level":"debug"}
	s.router.PUT("/internal/log/level", gin.WrapH(logger.LevelHandler()))

	// MCP — only registered when an API key is configured
	if s.mcpServer != nil {
		mcpGroup := s.router.Group("/mcp")
		s.mcpServer.RegisterRoutes(mcpGroup, s.mcpAPIKey)
	}

	if s.staticFS != nil {
		sub, _ := fs.Sub(s.staticFS, "dist")
		httpFS := http.FS(sub)

		// Read index.html once at startup for the SPA fallback
		indexHTML, _ := fs.ReadFile(sub, "index.html")

		s.router.NoRoute(func(c *gin.Context) {
			path := strings.TrimPrefix(c.Request.URL.Path, "/")
			if path != "" {
				f, err := sub.Open(path)
				if err == nil {
					f.Close()
					c.FileFromFS(path, httpFS)
					return
				}
			}
			// Serve index.html directly — must NOT use c.FileFromFS("index.html", ...)
			// because http.FileServer redirects any URL ending in /index.html to ./,
			// causing an infinite redirect loop.
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
		})
	}
}

func (s *Server) Run(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
