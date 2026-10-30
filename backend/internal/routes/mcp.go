package routes

import (
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/rpc"
)

func (s *Server) registerMCPRoutes() {
	if s.BeeRPC == nil {
		return
	}
	handler := s.BeeRPC.MCPHandler()
	for _, method := range []string{"POST", "GET", "DELETE"} {
		s.router.Handle(method, config.MCPPath, s.RPCAuthMiddleware, rpc.RequireBeeOrWorker(), handler)
	}
}
