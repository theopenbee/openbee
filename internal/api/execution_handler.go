package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Server) listWorkerExecutions(c *gin.Context) {
	workerID := c.Param("id")
	execs, err := s.ExecutionStore.ListByWorkerID(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (s *Server) listExecutions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	total, err := s.ExecutionStore.CountSessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	offset := (page - 1) * pageSize
	execs, err := s.ExecutionStore.ListPaginated(pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     execs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Server) getExecution(c *gin.Context) {
	exec, err := s.ExecutionStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	c.JSON(http.StatusOK, exec)
}

func (s *Server) listSessionExecutions(c *gin.Context) {
	sessionID := c.Param("sessionId")
	execs, err := s.ExecutionStore.ListBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (s *Server) getExecutionLogs(c *gin.Context) {
	id := c.Param("id")

	if content, ok := s.LogRegistry.Get(id); ok {
		c.String(http.StatusOK, content)
		return
	}

	content, err := s.ExecutionStore.ReadLog(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Only cache non-empty logs — an empty body means the file isn't written yet.
	if content != "" {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.String(http.StatusOK, content)
}
