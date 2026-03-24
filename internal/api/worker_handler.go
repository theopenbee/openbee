package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type createWorkerRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Memory      string `json:"memory"`
	WorkDir     string `json:"work_dir"`
}

func (s *Server) createWorker(c *gin.Context) {
	var req createWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	w, err := s.Manager.CreateWorker(
		req.Name, req.Description, req.Memory, req.WorkDir,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, w)
}

func (s *Server) listWorkers(c *gin.Context) {
	workers, err := s.WorkerStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workers)
}

func (s *Server) getWorker(c *gin.Context) {
	w, err := s.WorkerStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	c.JSON(http.StatusOK, w)
}

func (s *Server) updateWorker(c *gin.Context) {
	w, err := s.WorkerStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Memory      string `json:"memory"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		w.Name = req.Name
	}
	if req.Description != "" {
		w.Description = req.Description
	}
	if req.Memory != "" {
		w.Memory = req.Memory
	}

	updated, err := s.WorkerStore.Update(w)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteWorker(c *gin.Context) {
	id := c.Param("id")
	deleteWorkDir := c.Query("delete_work_dir") == "true"
	if err := s.Manager.DeleteWorker(id, deleteWorkDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
