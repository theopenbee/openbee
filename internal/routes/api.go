package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

func (s *Server) registerAuthRoutes() {
	authGroup := s.router.Group("/api/auth")
	authGroup.POST("/login", s.Auth.Login)
	authGroup.POST("/refresh", s.Auth.Refresh)
}

func (s *Server) registerAPIRoutes(r *gin.RouterGroup) {
	rp := func(perm string) gin.HandlerFunc { return auth.RequirePermission(s.Resolver, perm) }

	// Current-user endpoints (any authenticated user)
	r.GET("/me", s.Auth.Me)
	r.POST("/me/password", s.Auth.ChangePassword)

	// Workers
	r.POST("/workers", rp(auth.PermContactsWrite), s.Workers.Create)
	r.GET("/workers", rp(auth.PermContactsRead), s.Workers.List)
	r.GET("/workers/random-name", rp(auth.PermContactsRead), s.Workers.RandomName)
	r.GET("/workers/:id", rp(auth.PermContactsRead), s.Workers.Get)
	r.PUT("/workers/:id", rp(auth.PermContactsWrite), s.Workers.Update)
	r.DELETE("/workers/:id", rp(auth.PermContactsWrite), s.Workers.Delete)

	// Sessions
	r.GET("/sessions", rp(auth.PermSessionsRead), s.Executions.List)
	r.GET("/sessions/:id", rp(auth.PermSessionsRead), s.Executions.GetSession)
	r.GET("/sessions/:id/logs", rp(auth.PermSessionsRead), s.Executions.GetLogs)

	// Tasks
	r.GET("/tasks", rp(auth.PermTasksRead), s.Tasks.List)
	r.DELETE("/tasks/:id", rp(auth.PermTasksWrite), s.Tasks.Cancel)
	r.POST("/workers/:id/tasks/cancel-all", rp(auth.PermTasksWrite), s.Tasks.CancelByWorker)

	// Departments
	r.POST("/departments", rp(auth.PermContactsWrite), s.Departments.Create)
	r.GET("/departments", rp(auth.PermContactsRead), s.Departments.List)
	r.GET("/departments/:id", rp(auth.PermContactsRead), s.Departments.Get)
	r.PUT("/departments/:id", rp(auth.PermContactsWrite), s.Departments.Update)
	r.DELETE("/departments/:id", rp(auth.PermContactsWrite), s.Departments.Delete)
	r.PUT("/workers/:id/departments", rp(auth.PermContactsWrite), s.Departments.SetWorkerDepartments)
	r.GET("/workers/:id/departments", rp(auth.PermContactsRead), s.Departments.GetWorkerDepartments)
	r.GET("/departments/:id/workers", rp(auth.PermContactsRead), s.Departments.GetDepartmentWorkers)

	// Local chat & messages — gated by a single chat:write capability
	// (view + send), decoupled from worker-management permissions.
	r.POST("/local/messages", rp(auth.PermChatWrite), s.LocalChat.SendMessage)
	r.GET("/local/messages", rp(auth.PermChatWrite), s.LocalChat.GetMessages)
	r.POST("/local/media", rp(auth.PermChatWrite), s.LocalChat.UploadMedia)
	r.GET("/local/media/:filename", rp(auth.PermChatWrite), s.LocalChat.ServeMedia)
	r.GET("/local/stream", rp(auth.PermChatWrite), s.LocalChat.StreamReplies)

	// Version (any authenticated user)
	r.GET("/version", s.Version.Get)

	// Stats
	r.GET("/stats/overview", rp(auth.PermStatsRead), s.Stats.GetOverview)
	r.GET("/stats/token-trend", rp(auth.PermStatsRead), s.Stats.GetTokenTrend)

	// Env
	r.GET("/envs", rp(auth.PermEnvRead), s.Envs.List)
	r.POST("/envs", rp(auth.PermEnvWrite), s.Envs.Create)
	r.PUT("/envs/:id", rp(auth.PermEnvWrite), s.Envs.Update)
	r.DELETE("/envs/:id", rp(auth.PermEnvWrite), s.Envs.Delete)

	// System configs
	r.GET("/system-configs", rp(auth.PermSystemConfigRead), s.SystemConfigs.Get)
	r.PUT("/system-configs/:key", rp(auth.PermSystemConfigWrite), s.SystemConfigs.Set)

	// User & role administration
	r.GET("/users", rp(auth.PermUsersManage), s.Users.List)
	r.POST("/users", rp(auth.PermUsersManage), s.Users.Create)
	r.PUT("/users/:id/profile", rp(auth.PermUsersManage), s.Users.UpdateProfile)
	r.PUT("/users/:id/roles", rp(auth.PermUsersManage), s.Users.SetRoles)
	r.PUT("/users/:id/status", rp(auth.PermUsersManage), s.Users.SetStatus)
	r.POST("/users/:id/password", rp(auth.PermUsersManage), s.Users.ResetPassword)
	r.DELETE("/users/:id", rp(auth.PermUsersManage), s.Users.Delete)

	r.GET("/permissions", rp(auth.PermRolesManage), s.Roles.Catalog)
	r.GET("/roles", rp(auth.PermRolesManage), s.Roles.List)
	r.POST("/roles", rp(auth.PermRolesManage), s.Roles.Create)
	r.PUT("/roles/:id", rp(auth.PermRolesManage), s.Roles.Update)
	r.DELETE("/roles/:id", rp(auth.PermRolesManage), s.Roles.Delete)
}
