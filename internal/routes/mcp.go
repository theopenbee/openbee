package routes

import (
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/rpc"
)

func (s *Server) registerMCPRoutes() {
	s.router.POST(config.MCPBeeBasePath+"/call",
		s.MCPAuthMiddleware,
		rpc.RequireBeeOrWorker(),
		s.BeeMCP.HandleCall,
	)
}
