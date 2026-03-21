package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) listWorkerExecutions(c *gin.Context) {
	workerID := c.Param("id")
	execs, err := s.executionStore.ListByWorkerID(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (s *Server) listExecutions(c *gin.Context) {
	execs, err := s.executionStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (s *Server) getExecution(c *gin.Context) {
	exec, err := s.executionStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	c.JSON(http.StatusOK, exec)
}

func (s *Server) listSessionExecutions(c *gin.Context) {
	sessionID := c.Param("sessionId")
	execs, err := s.executionStore.ListBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (s *Server) getExecutionLogs(c *gin.Context) {
	id := c.Param("id")

	if content, ok := s.logRegistry.Get(id); ok {
		c.String(http.StatusOK, content)
		return
	}

	content, err := s.executionStore.ReadLog(id)
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
