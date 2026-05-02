package routes

import (
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/rpc"
)

func (s *Server) registerRPCRoutes() {
	s.router.POST(config.RPCBeeBasePath+"/call",
		s.RPCAuthMiddleware,
		rpc.RequireBeeOrWorker(),
		s.BeeRPC.HandleCall,
	)
}
