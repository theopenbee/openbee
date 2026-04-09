package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/mcp"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/domain/worker"
)

type ServerParams struct {
	WorkerStore      *store.WorkerStore
	ExecutionStore   *store.ExecutionStore
	TaskStore        *store.TaskStore
	DepartmentStore  *store.DepartmentStore
	StatsStore       *store.StatsStore
	Manager          *worker.Manager
	BeeMCPServer     *mcp.MCPServer
	TokenSecret string
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
		"/api/local/stream",
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
		s.registerTaskRoutes(api)
		s.registerDepartmentRoutes(api)
		s.registerLocalChatRoutes(api)
		s.registerStatsRoutes(api)
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
	api.POST("/local/messages", s.LocalChatHandler.sendMessage)
	api.GET("/local/messages", s.LocalChatHandler.getMessages)
	api.POST("/local/media", s.LocalChatHandler.uploadMedia)
	api.GET("/local/media/:filename", s.LocalChatHandler.serveMedia)
	api.GET("/local/stream", s.LocalChatHandler.StreamReplies)
}

func (s *Server) registerDepartmentRoutes(api *gin.RouterGroup) {
	api.POST("/departments", s.createDepartment)
	api.GET("/departments", s.listDepartments)
	api.GET("/departments/:id", s.getDepartment)
	api.PUT("/departments/:id", s.updateDepartment)
	api.DELETE("/departments/:id", s.deleteDepartment)
	api.PUT("/workers/:id/departments", s.setWorkerDepartments)
	api.GET("/workers/:id/departments", s.getWorkerDepartments)
	api.GET("/departments/:id/workers", s.getDepartmentWorkers)
}

func (s *Server) registerMCPRoutes() {
	s.router.POST(config.MCPBeeBasePath+"/call",
		mcp.JWTAuthMiddleware(s.TokenSecret),
		mcp.RequireBeeOrWorker(),
		s.BeeMCPServer.HandleCall,
	)
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
