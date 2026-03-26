package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/theopenbee/openbee/internal/skill"
)

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

func (s *Server) createSkill(c *gin.Context) {
	var req createSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	version, err := s.SkillManager.Create(req.Name, req.Description, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"name": req.Name, "version": version})
}

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

func (s *Server) createSkillVersion(c *gin.Context) {
	var req createVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	version, err := s.SkillManager.Edit(c.Param("name"), req.Content)
	if err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"latest_version": version})
}

func (s *Server) getSkillVersionContent(c *gin.Context) {
	name := c.Param("name")
	version := c.Param("version")
	content, err := s.SkillManager.ReadVersion(name, version)
	if err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

type setVersionRequest struct {
	Version string `json:"version" binding:"required"`
}

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

func (s *Server) adoptSkill(c *gin.Context) {
	version, err := s.SkillManager.AdoptGlobal(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "adopted", "version": version})
}

func (s *Server) resolveWorkerDir(c *gin.Context, workerID string) (string, bool) {
	w, err := s.WorkerStore.GetByID(workerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return "", false
	}
	return w.WorkDir, true
}

func (s *Server) listWorkerSkills(c *gin.Context) {
	workerID := c.Param("id")
	workDir, ok := s.resolveWorkerDir(c, workerID)
	if !ok {
		return
	}
	skills, err := s.SkillManager.ListWorker(workerID, workDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, skills)
}

func (s *Server) setWorkerSkillVersion(c *gin.Context) {
	var req setVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	workerID := c.Param("id")
	workDir, ok := s.resolveWorkerDir(c, workerID)
	if !ok {
		return
	}
	if err := s.SkillManager.UseWorker(workerID, workDir, c.Param("name"), req.Version); err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": req.Version})
}

func (s *Server) deleteWorkerSkillOverride(c *gin.Context) {
	workerID := c.Param("id")
	workDir, ok := s.resolveWorkerDir(c, workerID)
	if !ok {
		return
	}
	if err := s.SkillManager.RemoveWorkerOverride(workerID, workDir, c.Param("name")); err != nil {
		c.JSON(skillHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "override removed"})
}

func skillHTTPStatus(err error) int {
	if errors.Is(err, skill.ErrSkillNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
