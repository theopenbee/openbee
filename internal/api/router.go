package api

import (
	"context"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/auth"
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
	logRegistry      *worker.ActiveLogRegistry
	mcpServer        *mcp.MCPServer
	mcpAPIKey        string
	staticFS         fs.FS
	localChatHandler *LocalChatHandler
	authHandler      *auth.AuthHandler
	jwtMiddleware    gin.HandlerFunc
}

func NewServer(
	ws *store.WorkerStore,
	es *store.ExecutionStore,
	mgr *worker.Manager,
	logRegistry *worker.ActiveLogRegistry,
	mcpSrv *mcp.MCPServer,
	mcpAPIKey string,
	staticFS fs.FS,
	localChat *LocalChatHandler,
	authHandler *auth.AuthHandler,
	jwtMiddleware gin.HandlerFunc,
) *Server {
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathsRegexs([]string{
		"/api/local/sessions/.+/stream",
		"/mcp/sse",
		"/mcp/messages",
	})))

	s := &Server{
		router:           router,
		workerStore:      ws,
		executionStore:   es,
		manager:          mgr,
		logRegistry:      logRegistry,
		mcpServer:        mcpSrv,
		mcpAPIKey:        mcpAPIKey,
		staticFS:         staticFS,
		localChatHandler: localChat,
		authHandler:      authHandler,
		jwtMiddleware:    jwtMiddleware,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Auth routes — always registered, never behind JWT
	authGroup := s.router.Group("/api/auth")
	authGroup.GET("/status", s.authHandler.Status)
	authGroup.POST("/login", s.authHandler.Login)
	authGroup.POST("/refresh", s.authHandler.Refresh)

	// API routes — protected by JWT
	api := s.router.Group("/api")
	api.Use(s.jwtMiddleware)
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
		api.GET("/executions/:id/logs", s.getExecutionLogs)

		// Local chat
		s.localChatHandler.RegisterRoutes(api)
	}

	// SSE stream for local chat — registered outside gzip middleware but still JWT-protected
	s.registerProtected("GET", "/api/local/sessions/:id/stream", s.localChatHandler.StreamReplies)

	// Internal log level control — also JWT-protected
	s.registerProtected("PUT", "/internal/log/level", gin.WrapH(logger.LevelHandler()))

	// MCP
	mcpGroup := s.router.Group("/mcp")
	s.mcpServer.RegisterRoutes(mcpGroup, s.mcpAPIKey)

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

// registerProtected registers a route with JWT middleware.
func (s *Server) registerProtected(method, path string, handler gin.HandlerFunc) {
	s.router.Handle(method, path, s.jwtMiddleware, handler)
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
