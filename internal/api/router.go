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
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/mcp"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/worker"
)

type ServerParams struct {
	WorkerStore      *store.WorkerStore
	ExecutionStore   *store.ExecutionStore
	Manager          *worker.Manager
	BeeMCPServer     *mcp.MCPServer
	WorkerMCPServer  *mcp.MCPServer
	BeeAPIKey        string
	WorkerAPIKey     string
	StaticFS         fs.FS
	LocalChatHandler *LocalChatHandler
	AuthHandler      *auth.AuthHandler
	JWTMiddleware    gin.HandlerFunc
	Language         string
}

type Server struct {
	router *gin.Engine
	httpServer *http.Server
	ServerParams
}

func NewServer(p ServerParams) (*Server, error) {
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPathsRegexs([]string{
		"/api/local/sessions/.+/stream",
		"/mcp/.*/sse",
		"/mcp/.*/messages",
	})))

	s := &Server{
		router:       router,
		ServerParams: p,
	}
	if err := s.setupRoutes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) setupRoutes() error {
	s.registerAuthRoutes()
	s.router.GET("/api/config", s.getConfig) // public — no JWT required

	api := s.router.Group("/api")
	api.Use(s.JWTMiddleware)
	{
		s.registerWorkerRoutes(api)
		s.registerExecutionRoutes(api)
		s.registerLocalChatRoutes(api)
	}

	s.registerMCPRoutes()

	return s.registerStaticRoutes()
}

func (s *Server) registerAuthRoutes() {
	auth := s.router.Group("/api/auth")
	auth.POST("/login", s.AuthHandler.Login)
	auth.POST("/refresh", s.AuthHandler.Refresh)
}

func (s *Server) registerWorkerRoutes(api *gin.RouterGroup) {
	api.POST("/workers", s.createWorker)
	api.GET("/workers", s.listWorkers)
	api.GET("/workers/:id", s.getWorker)
	api.PUT("/workers/:id", s.updateWorker)
	api.DELETE("/workers/:id", s.deleteWorker)
}

func (s *Server) registerExecutionRoutes(api *gin.RouterGroup) {
	api.GET("/workers/:id/executions", s.listWorkerExecutions)
	api.GET("/sessions/:sessionId/executions", s.listSessionExecutions)
	api.GET("/executions", s.listExecutions)
	api.GET("/executions/:id", s.getExecution)
	api.GET("/executions/:id/logs", s.getExecutionLogs)
}

func (s *Server) registerLocalChatRoutes(api *gin.RouterGroup) {
	api.POST("/local/sessions", s.LocalChatHandler.createSession)
	api.GET("/local/sessions", s.LocalChatHandler.listSessions)
	api.DELETE("/local/sessions/:id", s.LocalChatHandler.deleteSession)
	api.POST("/local/sessions/:id/messages", s.LocalChatHandler.sendMessage)
	api.GET("/local/sessions/:id/messages", s.LocalChatHandler.getMessages)
	api.POST("/local/sessions/:id/media", s.LocalChatHandler.uploadMedia)
	api.GET("/local/sessions/:id/stream", s.LocalChatHandler.StreamReplies)
}

func (s *Server) registerMCPRoutes() {
	beeGroup := s.router.Group(config.MCPBeeBasePath)
	beeGroup.Use(mcp.APIKeyMiddleware(s.BeeAPIKey))
	beeGroup.GET("/sse", s.BeeMCPServer.HandleSSE)
	beeGroup.POST("/messages", s.BeeMCPServer.HandleMessages)

	workerGroup := s.router.Group(config.MCPWorkerBasePath)
	workerGroup.Use(mcp.APIKeyMiddleware(s.WorkerAPIKey))
	workerGroup.GET("/sse", s.WorkerMCPServer.HandleSSE)
	workerGroup.POST("/messages", s.WorkerMCPServer.HandleMessages)
}

func (s *Server) registerStaticRoutes() error {
	sub, err := fs.Sub(s.StaticFS, "dist")
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
