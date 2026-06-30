package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func respondWorkerError(c *gin.Context, err error) {
	respondDomainError(c, err, worker.ErrNotFound, worker.ErrValidation, "worker_not_found", "worker_validation")
}

type createWorkerRequest struct {
	Name             string            `json:"name" binding:"required"`
	Engine           string            `json:"engine"`
	Description      string            `json:"description"`
	Constraints      string            `json:"constraints"`
	WorkDir          string            `json:"work_dir"`
	PermissionScopes string            `json:"permission_scopes"`
	EngineArgs       map[string]string `json:"engine_args"`
}

type workerResponse struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Constraints      string             `json:"constraints"`
	WorkDir          string             `json:"work_dir"`
	Engine           string             `json:"engine"`
	EngineArgs       map[string]string  `json:"engine_args"`
	Status           model.WorkerStatus `json:"status"`
	PermissionScopes string             `json:"permission_scopes"`
	CreatedAt        int64              `json:"created_at"`
	UpdatedAt        int64              `json:"updated_at"`
}

type workerWithDepartmentsResponse struct {
	workerResponse
	Departments []model.DepartmentBrief `json:"departments"`
}

func parseWorkerEngineArgs(raw string) (map[string]string, error) {
	if raw == "" || raw == "{}" {
		return map[string]string{}, nil
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func toWorkerResponse(w model.Worker) (workerResponse, error) {
	engineArgs, err := parseWorkerEngineArgs(w.EngineArgs)
	if err != nil {
		return workerResponse{}, fmt.Errorf("parse worker %s engine_args: %w", w.ID, err)
	}
	return workerResponse{
		ID:               w.ID,
		Name:             w.Name,
		Description:      w.Description,
		Constraints:      w.Constraints,
		WorkDir:          w.WorkDir,
		Engine:           w.Engine,
		EngineArgs:       engineArgs,
		Status:           w.Status,
		PermissionScopes: w.PermissionScopes,
		CreatedAt:        w.CreatedAt,
		UpdatedAt:        w.UpdatedAt,
	}, nil
}

func toWorkerWithDepartmentsResponse(w model.Worker, depts []model.DepartmentBrief) (workerWithDepartmentsResponse, error) {
	resp, err := toWorkerResponse(w)
	if err != nil {
		return workerWithDepartmentsResponse{}, err
	}
	return workerWithDepartmentsResponse{workerResponse: resp, Departments: depts}, nil
}

type WorkerHandler struct {
	workers     *store.WorkerStore
	departments *store.DepartmentStore
	manager     *worker.Manager
	language    string
}

func NewWorkerHandler(ws *store.WorkerStore, ds *store.DepartmentStore, mgr *worker.Manager, lang string) *WorkerHandler {
	return &WorkerHandler{workers: ws, departments: ds, manager: mgr, language: lang}
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

	if err := h.manager.ValidateEngine(req.Engine); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.ValidateEngineArgs(req.EngineArgs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var engineArgsJSON string
	if len(req.EngineArgs) > 0 {
		b, _ := json.Marshal(req.EngineArgs)
		engineArgsJSON = string(b)
	} else {
		engineArgsJSON = "{}"
	}

	w, err := h.manager.CreateWorker(worker.CreateWorkerParams{
		Name:             req.Name,
		Engine:           req.Engine,
		Description:      req.Description,
		Constraints:      req.Constraints,
		WorkDir:          req.WorkDir,
		PermissionScopes: req.PermissionScopes,
		EngineArgs:       engineArgsJSON,
	})
	if err != nil {
		respondWorkerError(c, err)
		return
	}
	resp, err := toWorkerResponse(w)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, resp)
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

	resp := make([]workerWithDepartmentsResponse, 0, len(workers))
	for _, w := range workers {
		item, convErr := toWorkerWithDepartmentsResponse(w, model.ToDepartmentBriefs(deptMap[w.ID]))
		if convErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": convErr.Error()})
			return
		}
		resp = append(resp, item)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkerHandler) Get(c *gin.Context) {
	w, err := h.workers.GetByID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, apperr.New("worker_not_found", "worker not found"))
		return
	}
	depts, err := h.departments.GetWorkerDepartments(w.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp, convErr := toWorkerWithDepartmentsResponse(w, model.ToDepartmentBriefs(depts))
	if convErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": convErr.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkerHandler) Update(c *gin.Context) {
	var req worker.UpdateWorkerParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	w, err := h.manager.UpdateWorker(c.Param("id"), req)
	if err != nil {
		respondWorkerError(c, err)
		return
	}
	resp, convErr := toWorkerResponse(w)
	if convErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": convErr.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
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

func (h *WorkerHandler) RandomName(c *gin.Context) {
	pool := worker.NamePool(h.language)
	names, err := h.workers.ListNames()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	used := make(map[string]struct{}, len(names))
	for _, n := range names {
		used[n] = struct{}{}
	}
	name, ok := worker.PickRandomName(pool, used)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"exhausted": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name})
}
