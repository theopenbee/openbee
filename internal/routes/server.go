package routes

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/rpc"
)

type ServerParams struct {
	Workers           *api.WorkerHandler
	Executions        *api.ExecutionHandler
	Messages          *api.MessageHandler
	Tasks             *api.TaskHandler
	Departments       *api.DepartmentHandler
	Stats             *api.StatsHandler
	Config            *api.ConfigHandler
	LocalChat         *api.LocalChatHandler
	Auth              *auth.AuthHandler
	Envs              *api.EnvHandler
	SystemConfigs     *api.SystemConfigHandler
	BeeMCP            *rpc.MCPServer
	MCPAuthMiddleware gin.HandlerFunc
	StaticFS          fs.FS
	JWTMiddleware     gin.HandlerFunc
}

type Server struct {
	router     *gin.Engine
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

	apiGroup := s.router.Group("/api")
	apiGroup.Use(s.JWTMiddleware)
	s.registerAPIRoutes(apiGroup)

	s.registerMCPRoutes()

	return s.registerStaticRoutes()
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
