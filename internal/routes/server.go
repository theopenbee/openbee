package routes

import (
	"context"
	"io/fs"
	"net/http"
	"sync"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/rpc"
)

type ServerParams struct {
	Workers           *api.WorkerHandler
	Executions        *api.ExecutionHandler
	Tasks             *api.TaskHandler
	Departments       *api.DepartmentHandler
	Stats             *api.StatsHandler
	Config            *api.ConfigHandler
	Version           *api.VersionHandler
	LocalChat         *api.LocalChatHandler
	Auth              *auth.AuthHandler
	Envs              *api.EnvHandler
	SystemConfigs     *api.SystemConfigHandler
	Users             *api.UserHandler
	Roles             *api.RoleHandler
	Setup             *api.SetupHandler
	BeeRPC            *rpc.Server
	RPCAuthMiddleware gin.HandlerFunc
	StaticFS          fs.FS
	AuthMiddleware    gin.HandlerFunc
	Resolver          *auth.PermissionResolver
}

type Server struct {
	router *gin.Engine

	// mu guards httpServer, which Run writes and Shutdown reads from a
	// different goroutine.
	mu         sync.Mutex
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
	s.router.GET("/api/config", s.Config.Get)
	s.router.GET("/api/setup/status", s.Setup.Status)
	s.router.POST("/api/setup", s.Setup.Create)

	apiGroup := s.router.Group("/api")
	apiGroup.Use(s.AuthMiddleware)
	s.registerAPIRoutes(apiGroup)

	s.registerRPCRoutes()

	return s.registerStaticRoutes()
}

func (s *Server) Run(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	s.mu.Lock()
	s.httpServer = srv
	s.mu.Unlock()
	return srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpServer
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
