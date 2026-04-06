package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type createDepartmentRequest struct {
	Name      string  `json:"name" binding:"required"`
	ParentID  *string `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

func (s *Server) createDepartment(c *gin.Context) {
	var req createDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	d, err := s.DepartmentStore.Create(model.Department{
		Name:      req.Name,
		ParentID:  req.ParentID,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (s *Server) listDepartments(c *gin.Context) {
	depts, err := s.DepartmentStore.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tree := s.DepartmentStore.BuildTree(depts)
	c.JSON(http.StatusOK, tree)
}

func (s *Server) getDepartment(c *gin.Context) {
	d, err := s.DepartmentStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (s *Server) updateDepartment(c *gin.Context) {
	d, err := s.DepartmentStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}

	var req struct {
		Name      *string `json:"name"`
		ParentID  *string `json:"parent_id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		d.Name = *req.Name
	}
	if req.ParentID != nil {
		if err := s.DepartmentStore.CheckCircularReference(d.ID, *req.ParentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		d.ParentID = req.ParentID
	}
	if req.SortOrder != nil {
		d.SortOrder = *req.SortOrder
	}

	updated, err := s.DepartmentStore.Update(d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteDepartment(c *gin.Context) {
	if err := s.DepartmentStore.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type setWorkerDepartmentsRequest struct {
	DepartmentIDs []string `json:"department_ids" binding:"required"`
}

func (s *Server) setWorkerDepartments(c *gin.Context) {
	workerID := c.Param("id")
	if _, err := s.WorkerStore.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	var req setWorkerDepartmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate all department IDs exist
	found, err := s.DepartmentStore.GetByIDs(req.DepartmentIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(found) != len(req.DepartmentIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "one or more departments not found"})
		return
	}

	if err := s.DepartmentStore.SetWorkerDepartments(workerID, req.DepartmentIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"department_ids": req.DepartmentIDs})
}

func (s *Server) getWorkerDepartments(c *gin.Context) {
	workerID := c.Param("id")
	if _, err := s.WorkerStore.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	depts, err := s.DepartmentStore.GetWorkerDepartments(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, depts)
}

func (s *Server) getDepartmentWorkers(c *gin.Context) {
	deptID := c.Param("id")
	workerIDs, err := s.DepartmentStore.GetDepartmentWorkerIDs(deptID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	workers, err := s.WorkerStore.GetByIDs(workerIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if workers == nil {
		workers = []model.Worker{}
	}
	c.JSON(http.StatusOK, workers)
}
