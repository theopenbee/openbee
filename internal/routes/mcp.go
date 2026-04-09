package routes

import (
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/mcp"
)

func (s *Server) registerMCPRoutes() {
	s.router.POST(config.MCPBeeBasePath+"/call",
		mcp.JWTAuthMiddleware(s.TokenSecret),
		mcp.RequireBeeOrWorker(),
		s.BeeMCP.HandleCall,
	)
}
