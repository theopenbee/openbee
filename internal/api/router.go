package api

import (
	"context"
	"fmt"
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
) (*Server, error) {
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
	if err := s.setupRoutes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) setupRoutes() error {
	authGroup := s.router.Group("/api/auth")
	authGroup.GET("/status", s.authHandler.Status)
	authGroup.POST("/login", s.authHandler.Login)
	authGroup.POST("/refresh", s.authHandler.Refresh)

	api := s.router.Group("/api")
	api.Use(s.jwtMiddleware)
	{
		api.POST("/workers", s.createWorker)
		api.GET("/workers", s.listWorkers)
		api.GET("/workers/:id", s.getWorker)
		api.PUT("/workers/:id", s.updateWorker)
		api.DELETE("/workers/:id", s.deleteWorker)
		api.GET("/workers/:id/executions", s.listWorkerExecutions)
		api.GET("/sessions/:sessionId/executions", s.listSessionExecutions)
		api.GET("/executions", s.listExecutions)
		api.GET("/executions/:id", s.getExecution)
		api.GET("/executions/:id/logs", s.getExecutionLogs)
		api.POST("/local/sessions", s.localChatHandler.createSession)
		api.GET("/local/sessions", s.localChatHandler.listSessions)
		api.DELETE("/local/sessions/:id", s.localChatHandler.deleteSession)
		api.POST("/local/sessions/:id/messages", s.localChatHandler.sendMessage)
		api.GET("/local/sessions/:id/messages", s.localChatHandler.getMessages)
		api.POST("/local/sessions/:id/media", s.localChatHandler.uploadMedia)
		api.GET("/local/sessions/:id/stream", s.localChatHandler.StreamReplies)
	}

	s.router.Handle("PUT", "/internal/log/level", s.jwtMiddleware, gin.WrapH(logger.LevelHandler()))

	mcpGroup := s.router.Group("/mcp")
	s.mcpServer.RegisterRoutes(mcpGroup, s.mcpAPIKey)

	sub, err := fs.Sub(s.staticFS, "dist")
	if err != nil {
		return fmt.Errorf("static assets: %w", err)
	}
	httpFS := http.FS(sub)

	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return fmt.Errorf("reading index.html: %w", err)
	}

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
	return nil
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
