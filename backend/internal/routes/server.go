package routes

import (
	"context"
	"io/fs"
	"net"
	"net/http"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/api"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/rpc"
)

type ServerParams struct {
	Workers           *api.WorkerHandler
	Config            *api.ConfigHandler
	LocalChat         *api.LocalChatHandler
	Auth              *auth.AuthHandler
	Envs              *api.EnvHandler
	SystemConfigs     *api.SystemConfigHandler
	Setup             *api.SetupHandler
	Plugins           *api.PluginHandler
	BeeRPC            *rpc.Server
	RPCAuthMiddleware gin.HandlerFunc
	StaticFS          fs.FS
	AuthMiddleware    gin.HandlerFunc
	PluginsAuth       gin.HandlerFunc
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
	}), gzip.WithExcludedPaths([]string{config.MCPPath})))

	s := &Server{
		router:       router,
		httpServer:   &http.Server{Handler: router},
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

	s.registerPluginRoutes(s.router.Group("/api/plugins", s.PluginsAuth))

	apiGroup := s.router.Group("/api")
	apiGroup.Use(s.AuthMiddleware)
	s.registerAPIRoutes(apiGroup)

	s.registerMCPRoutes()

	return s.registerStaticRoutes()
}

func (s *Server) Serve(ln net.Listener) error {
	return s.httpServer.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
