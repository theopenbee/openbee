package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type workerResponse struct {
	model.Worker
	Departments []departmentBrief `json:"departments"`
}

type departmentBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

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

	// Filter by department_id if provided
	deptID := c.Query("department_id")

	var result []workerResponse
	for _, w := range workers {
		depts, _ := s.DepartmentStore.GetWorkerDepartments(w.ID)
		if deptID != "" {
			found := false
			for _, d := range depts {
				if d.ID == deptID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		briefs := make([]departmentBrief, 0, len(depts))
		for _, d := range depts {
			briefs = append(briefs, departmentBrief{ID: d.ID, Name: d.Name})
		}
		result = append(result, workerResponse{Worker: w, Departments: briefs})
	}
	if result == nil {
		result = []workerResponse{}
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) getWorker(c *gin.Context) {
	w, err := s.WorkerStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	depts, _ := s.DepartmentStore.GetWorkerDepartments(w.ID)
	briefs := make([]departmentBrief, 0, len(depts))
	for _, d := range depts {
		briefs = append(briefs, departmentBrief{ID: d.ID, Name: d.Name})
	}
	c.JSON(http.StatusOK, workerResponse{Worker: w, Departments: briefs})
}

func (s *Server) updateWorker(c *gin.Context) {
	w, err := s.WorkerStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Memory      *string `json:"memory"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.Description != nil {
		w.Description = *req.Description
	}
	if req.Memory != nil {
		w.Memory = *req.Memory
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
	s.DepartmentStore.DeleteWorkerDepartments(id)
	if err := s.Manager.DeleteWorker(id, deleteWorkDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
