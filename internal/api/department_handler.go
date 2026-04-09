package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type createDepartmentRequest struct {
	Name      string  `json:"name" binding:"required"`
	ParentID  *string `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

type DepartmentHandler struct {
	departments *store.DepartmentStore
	workers     *store.WorkerStore
}

func NewDepartmentHandler(ds *store.DepartmentStore, ws *store.WorkerStore) *DepartmentHandler {
	return &DepartmentHandler{departments: ds, workers: ws}
}

func (h *DepartmentHandler) Create(c *gin.Context) {
	var req createDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	d, err := h.departments.Create(model.Department{
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

func (h *DepartmentHandler) List(c *gin.Context) {
	depts, err := h.departments.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tree := h.departments.BuildTree(depts)
	c.JSON(http.StatusOK, tree)
}

func (h *DepartmentHandler) Get(c *gin.Context) {
	d, err := h.departments.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *DepartmentHandler) Update(c *gin.Context) {
	d, err := h.departments.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
		return
	}

	var req struct {
		Name      *string         `json:"name"`
		ParentID  json.RawMessage `json:"parent_id"`
		SortOrder *int            `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		d.Name = *req.Name
	}
	if req.ParentID != nil {
		if string(req.ParentID) == "null" {
			d.ParentID = nil
		} else {
			var parentID string
			if err := json.Unmarshal(req.ParentID, &parentID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parent_id"})
				return
			}
			if err := h.departments.CheckCircularReference(d.ID, parentID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			d.ParentID = &parentID
		}
	}
	if req.SortOrder != nil {
		d.SortOrder = *req.SortOrder
	}

	updated, err := h.departments.Update(d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *DepartmentHandler) Delete(c *gin.Context) {
	if err := h.departments.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type setWorkerDepartmentsRequest struct {
	DepartmentIDs []string `json:"department_ids" binding:"required"`
}

func (h *DepartmentHandler) SetWorkerDepartments(c *gin.Context) {
	workerID := c.Param("id")
	if _, err := h.workers.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	var req setWorkerDepartmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	found, err := h.departments.GetByIDs(req.DepartmentIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(found) != len(req.DepartmentIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "one or more departments not found"})
		return
	}

	if err := h.departments.SetWorkerDepartments(workerID, req.DepartmentIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"department_ids": req.DepartmentIDs})
}

func (h *DepartmentHandler) GetWorkerDepartments(c *gin.Context) {
	workerID := c.Param("id")
	if _, err := h.workers.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}

	depts, err := h.departments.GetWorkerDepartments(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, depts)
}

func (h *DepartmentHandler) GetDepartmentWorkers(c *gin.Context) {
	deptID := c.Param("id")
	workers, err := h.workers.GetByDepartmentID(deptID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if workers == nil {
		workers = []model.Worker{}
	}
	c.JSON(http.StatusOK, workers)
}
