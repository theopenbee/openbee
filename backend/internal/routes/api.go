package routes

import (
	"github.com/gin-gonic/gin"
)

func (s *Server) registerAuthRoutes() {
	authGroup := s.router.Group("/api/auth")
	authGroup.POST("/login", s.Auth.Login)
	authGroup.POST("/refresh", s.Auth.Refresh)
}

func (s *Server) registerAPIRoutes(r *gin.RouterGroup) {
	// Current-user endpoints
	r.GET("/me", s.Auth.Me)
	r.POST("/me/password", s.Auth.ChangePassword)

	// Workers
	r.POST("/workers", s.Workers.Create)
	r.GET("/workers", s.Workers.List)
	r.GET("/workers/random-name", s.Workers.RandomName)
	r.GET("/workers/:id", s.Workers.Get)
	r.PUT("/workers/:id", s.Workers.Update)
	r.DELETE("/workers/:id", s.Workers.Delete)

	// Local chat & messages
	r.GET("/local/conversations", s.LocalChat.ListConversations)
	r.GET("/local/conversations/:cid/messages", s.LocalChat.GetMessages)
	r.POST("/local/conversations/:cid/messages", s.LocalChat.SendMessage)
	r.POST("/local/conversations/:cid/read", s.LocalChat.MarkRead)
	r.POST("/local/conversations/:cid/clear", s.LocalChat.Clear)
	r.POST("/local/conversations/:cid/stop", s.LocalChat.Stop)
	r.POST("/local/media", s.LocalChat.UploadMedia)
	r.GET("/local/media/:filename", s.LocalChat.ServeMedia)
	r.GET("/local/stream", s.LocalChat.StreamReplies)

	// Env
	r.GET("/envs", s.Envs.List)
	r.POST("/envs", s.Envs.Create)
	r.PUT("/envs/:id", s.Envs.Update)
	r.DELETE("/envs/:id", s.Envs.Delete)

	// System configs
	r.GET("/system-configs", s.SystemConfigs.Get)
	r.PUT("/system-configs/:key", s.SystemConfigs.Set)
}

func (s *Server) registerPluginRoutes(r *gin.RouterGroup) {
	r.GET("", s.Plugins.List)
	r.GET("/catalog", s.Plugins.Catalog)
	r.POST("/install", s.Plugins.Install)
	r.GET("/operations", s.Plugins.Operations)
	r.GET("/operations/:id", s.Plugins.Operation)
	r.GET("/:id", s.Plugins.Get)
	r.PUT("/:id/settings", s.Plugins.UpdateSettings)
	r.POST("/:id/enable", s.Plugins.Enable)
	r.POST("/:id/disable", s.Plugins.Disable)
	r.POST("/:id/restart", s.Plugins.Restart)
	r.DELETE("/:id", s.Plugins.Uninstall)
	r.POST("/:id/setup", s.Plugins.StartSetup)
	r.GET("/setup/:sid/events", s.Plugins.SetupEvents)
	r.POST("/setup/:sid/input", s.Plugins.SetupInput)
	r.DELETE("/setup/:sid", s.Plugins.CancelSetup)
}
