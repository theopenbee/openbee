package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/theopenbee/openbee/internal/skill"
)

// listSkills GET /api/skills
func (s *Server) listSkills(c *gin.Context) {
	skills, err := s.SkillManager.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, skills)
}

type createSkillRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Content     string `json:"content" binding:"required"`
}

// createSkill POST /api/skills
func (s *Server) createSkill(c *gin.Context) {
	var req createSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.SkillManager.Create(req.Name, req.Description, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"name": req.Name, "version": "v1"})
}

// getSkill GET /api/skills/:name
func (s *Server) getSkill(c *gin.Context) {
	name := c.Param("name")
	cfg, err := s.SkillManager.LoadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	entry, ok := cfg.Skills[name]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	c.JSON(http.StatusOK, entry)
}

// deleteSkill DELETE /api/skills/:name
func (s *Server) deleteSkill(c *gin.Context) {
	if err := s.SkillManager.Delete(c.Param("name")); err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type createVersionRequest struct {
	Content string `json:"content" binding:"required"`
}

// createSkillVersion POST /api/skills/:name/versions
func (s *Server) createSkillVersion(c *gin.Context) {
	var req createVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.SkillManager.Edit(c.Param("name"), req.Content); err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	cfg, err := s.SkillManager.LoadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	entry := cfg.Skills[c.Param("name")]
	c.JSON(http.StatusCreated, gin.H{"latest_version": entry.LatestVersion})
}

type setVersionRequest struct {
	Version string `json:"version" binding:"required"`
}

// setGlobalVersion PUT /api/skills/:name/global-version
func (s *Server) setGlobalVersion(c *gin.Context) {
	var req setVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.SkillManager.UseGlobal(c.Param("name"), req.Version); err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"global_version": req.Version})
}

// adoptSkill POST /api/skills/:name/adopt
func (s *Server) adoptSkill(c *gin.Context) {
	if err := s.SkillManager.AdoptGlobal(c.Param("name")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "adopted", "version": "v1"})
}

// listWorkerSkills GET /api/workers/:id/skills
func (s *Server) listWorkerSkills(c *gin.Context) {
	workerID := c.Param("id")
	w, err := s.WorkerStore.GetByID(workerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	skills, err := s.SkillManager.ListWorker(workerID, w.WorkDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, skills)
}

// setWorkerSkillVersion PUT /api/workers/:id/skills/:name
func (s *Server) setWorkerSkillVersion(c *gin.Context) {
	var req setVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	workerID := c.Param("id")
	w, err := s.WorkerStore.GetByID(workerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	if err := s.SkillManager.UseWorker(workerID, w.WorkDir, c.Param("name"), req.Version); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": req.Version})
}

// deleteWorkerSkillOverride DELETE /api/workers/:id/skills/:name
func (s *Server) deleteWorkerSkillOverride(c *gin.Context) {
	workerID := c.Param("id")
	w, err := s.WorkerStore.GetByID(workerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return
	}
	if err := s.SkillManager.RemoveWorkerOverride(workerID, w.WorkDir, c.Param("name")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "override removed"})
}

// skillHTTPStatus returns 404 for skill-not-found errors, 500 otherwise.
func skillHTTPStatus(err error) int {
	if errors.Is(err, skill.ErrSkillNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
