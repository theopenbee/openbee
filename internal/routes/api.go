package routes

import "github.com/gin-gonic/gin"

func (s *Server) registerAuthRoutes() {
	auth := s.router.Group("/api/auth")
	auth.POST("/login", s.Auth.Login)
	auth.POST("/refresh", s.Auth.Refresh)
}

func (s *Server) registerAPIRoutes(r *gin.RouterGroup) {
	r.POST("/workers", s.Workers.Create)
	r.GET("/workers", s.Workers.List)
	r.GET("/workers/random-name", s.Workers.RandomName)
	r.GET("/workers/:id", s.Workers.Get)
	r.PUT("/workers/:id", s.Workers.Update)
	r.DELETE("/workers/:id", s.Workers.Delete)

	r.GET("/sessions", s.Executions.List)
	r.GET("/sessions/:id", s.Executions.GetSession)
	r.GET("/sessions/:id/logs", s.Executions.GetLogs)

	r.GET("/tasks", s.Tasks.List)
	r.DELETE("/tasks/:id", s.Tasks.Cancel)
	r.POST("/workers/:id/tasks/cancel-all", s.Tasks.CancelByWorker)

	r.POST("/departments", s.Departments.Create)
	r.GET("/departments", s.Departments.List)
	r.GET("/departments/:id", s.Departments.Get)
	r.PUT("/departments/:id", s.Departments.Update)
	r.DELETE("/departments/:id", s.Departments.Delete)
	r.PUT("/workers/:id/departments", s.Departments.SetWorkerDepartments)
	r.GET("/workers/:id/departments", s.Departments.GetWorkerDepartments)
	r.GET("/departments/:id/workers", s.Departments.GetDepartmentWorkers)

	// Group routes
	r.POST("/groups", s.Groups.Create)
	r.GET("/groups", s.Groups.List)
	r.GET("/groups/:id", s.Groups.Get)
	r.PUT("/groups/:id", s.Groups.Update)
	r.DELETE("/groups/:id", s.Groups.Delete)
	r.GET("/groups/:id/members", s.Groups.ListMembers)
	r.POST("/groups/:id/members", s.Groups.AddMember)
	r.DELETE("/groups/:id/members/:worker_id", s.Groups.RemoveMember)

	// Subtask routes (used by Group agent CLI calls)
	r.POST("/tasks/dispatch-subtask", s.Subtasks.Dispatch)
	r.GET("/tasks/subtasks", s.Subtasks.ListSubtasks)
	r.POST("/tasks/suspend", s.Subtasks.Suspend)
	r.POST("/tasks/mark-success", s.Subtasks.MarkSuccess)
	r.POST("/tasks/mark-failed", s.Subtasks.MarkFailed)

	r.POST("/local/messages", s.LocalChat.SendMessage)
	r.GET("/local/messages", s.LocalChat.GetMessages)
	r.POST("/local/media", s.LocalChat.UploadMedia)
	r.GET("/local/media/:filename", s.LocalChat.ServeMedia)
	r.GET("/local/stream", s.LocalChat.StreamReplies)

	r.GET("/messages", s.Messages.List)

	r.GET("/stats/overview", s.Stats.GetOverview)
	r.GET("/stats/trend", s.Stats.GetTrend)
	r.GET("/stats/execution-duration-trend", s.Stats.GetExecutionDurationTrend)
	r.GET("/stats/token-trend", s.Stats.GetTokenTrend)

	r.GET("/envs", s.Envs.List)
	r.POST("/envs", s.Envs.Create)
	r.PUT("/envs/:id", s.Envs.Update)
	r.DELETE("/envs/:id", s.Envs.Delete)

	r.GET("/system-configs", s.SystemConfigs.Get)
	r.PUT("/system-configs/:key", s.SystemConfigs.Set)
}
