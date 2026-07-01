package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/apperr"
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
		respondError(c, http.StatusBadRequest, err)
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

func (h *DepartmentHandler) Update(c *gin.Context) {
	d, err := h.departments.GetByID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, apperr.New("department_not_found", "department not found"))
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
				respondError(c, http.StatusBadRequest, apperr.New("department_invalid_parent", "invalid parent_id"))
				return
			}
			if err := h.departments.CheckCircularReference(d.ID, parentID); err != nil {
				respondError(c, http.StatusBadRequest, err)
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
		respondError(c, http.StatusBadRequest, err)
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
		respondError(c, http.StatusNotFound, apperr.New("worker_not_found", "worker not found"))
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
		respondError(c, http.StatusBadRequest, apperr.New("departments_not_found", "one or more departments not found"))
		return
	}

	if err := h.departments.SetWorkerDepartments(workerID, req.DepartmentIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"department_ids": req.DepartmentIDs})
}
