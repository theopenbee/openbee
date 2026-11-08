//go:build !embedserver

package localserver

import (
	"context"
	"time"
)

const Bundled = false

type Server struct{}

func RunChild() {}

func New(Options) *Server {
	return nil
}

func (*Server) URL() string {
	return ""
}

func (*Server) Start(context.Context) error {
	return nil
}

func (*Server) Stop(time.Duration) {}
