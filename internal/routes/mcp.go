package routes

import (
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/mcp"
)

func (s *Server) registerMCPRoutes() {
	s.router.POST(config.MCPBeeBasePath+"/call",
		s.MCPAuthMiddleware,
		mcp.RequireBeeOrWorker(),
		s.BeeMCP.HandleCall,
	)
}
