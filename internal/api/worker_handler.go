package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type createWorkerRequest struct {
	Name             string `json:"name" binding:"required"`
	Engine           string `json:"engine"`
	Description      string `json:"description"`
	Memory           string `json:"memory"`
	WorkDir          string `json:"work_dir"`
	PermissionScopes string `json:"permission_scopes"`
}

type WorkerHandler struct {
	workers     *store.WorkerStore
	departments *store.DepartmentStore
	manager     *worker.Manager
}

func NewWorkerHandler(ws *store.WorkerStore, ds *store.DepartmentStore, mgr *worker.Manager) *WorkerHandler {
	return &WorkerHandler{workers: ws, departments: ds, manager: mgr}
}

func (h *WorkerHandler) Create(c *gin.Context) {
	var req createWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := auth.ValidatePermissionScopes(req.PermissionScopes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Engine != "" {
		if err := ai.ValidateEngine(req.Engine); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	w, err := h.manager.CreateWorker(worker.CreateWorkerParams{
		Name:             req.Name,
		Engine:           req.Engine,
		Description:      req.Description,
		Memory:           req.Memory,
		WorkDir:          req.WorkDir,
		PermissionScopes: req.PermissionScopes,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, w)
}

func (h *WorkerHandler) List(c *gin.Context) {
	deptID := c.Query("department_id")

	var workers []model.Worker
	var err error
	if deptID != "" {
		workers, err = h.workers.GetByDepartmentID(deptID)
	} else {
		workers, err = h.workers.List()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	workerIDs := make([]string, len(workers))
	for i, w := range workers {
		workerIDs[i] = w.ID
	}
	deptMap, err := h.departments.GetWorkersDepartments(workerIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]model.WorkerWithDepartments, 0, len(workers))
	for _, w := range workers {
		result = append(result, model.WorkerWithDepartments{Worker: w, Departments: model.ToDepartmentBriefs(deptMap[w.ID])})
	}
	c.JSON(http.StatusOK, result)
}

func (h *WorkerHandler) Get(c *gin.Context) {
	w, err := h.workers.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	depts, err := h.departments.GetWorkerDepartments(w.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, model.WorkerWithDepartments{Worker: w, Departments: model.ToDepartmentBriefs(depts)})
}

func (h *WorkerHandler) Update(c *gin.Context) {
	w, err := h.workers.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		Memory           *string `json:"memory"`
		PermissionScopes *string `json:"permission_scopes"`
		Engine           *string `json:"engine"`
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
	if req.PermissionScopes != nil {
		if err := auth.ValidatePermissionScopes(*req.PermissionScopes); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		w.PermissionScopes = *req.PermissionScopes
	}

	if req.Engine != nil {
		if err := ai.ValidateEngine(*req.Engine); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		w.Engine = *req.Engine
	}

	updated, err := h.workers.Update(w)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (h *WorkerHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	deleteWorkDir := c.Query("delete_work_dir") == "true"
	if err := h.manager.DeleteWorker(id, deleteWorkDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.departments.DeleteWorkerDepartments(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
