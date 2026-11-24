package fakeapi

import (
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func (s *Server) onboardingRoutes() {
	s.route("GET /api/onboarding", authUser, s.getOnboarding)
	s.route("POST /api/onboarding/skip", authUser, s.skipOnboarding)
}

func (s *Server) getOnboarding(*request) reply {
	engine := slices.ContainsFunc(s.st.Plugins, func(p api.PluginDetail) bool { return p.Type == typeEngine })
	return ok(api.OnboardingStatus{Pending: !s.st.OnboardingSkipped && !engine})
}

func (s *Server) skipOnboarding(*request) reply {
	s.st.OnboardingSkipped = true
	return noContent()
}
