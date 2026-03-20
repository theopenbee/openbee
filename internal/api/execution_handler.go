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
