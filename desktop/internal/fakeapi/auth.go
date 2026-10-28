package fakeapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	accessTTL         = 2 * time.Hour
	minPasswordLength = 6
	userID            = "user-1"
)

type tokenBook struct {
	seq     int
	access  map[string]bool
	refresh map[string]bool
}

func newTokenBook() tokenBook {
	return tokenBook{access: map[string]bool{}, refresh: map[string]bool{}}
}

type account struct {
	displayName string
	createdAt   int64
}

func accessToken(n int, exp time.Time) string {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"type": "access", "exp": exp.Unix(), "n": n})
	return header + "." + enc.EncodeToString(payload) + ".fake"
}

func (s *Server) issueAccess() string {
	s.tokens.seq++
	token := accessToken(s.tokens.seq, time.Now().Add(accessTTL))
	s.tokens.access[token] = true
	return token
}

func (s *Server) issuePair() api.TokenPair {
	s.tokens.seq++
	refresh := fmt.Sprintf("refresh-%d", s.tokens.seq)
	s.tokens.refresh[refresh] = true
	return api.TokenPair{
		AccessToken:  s.issueAccess(),
		RefreshToken: refresh,
		ExpiresIn:    int64(accessTTL / time.Second),
	}
}

func (s *Server) ExpireAccessTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.tokens.access)
}

func (s *Server) RevokeRefreshTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.tokens.refresh)
}

func bearerToken(r *http.Request) string {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return token
	}
	return r.URL.Query().Get("token")
}

func unauthorized() reply {
	return errReply(http.StatusUnauthorized, "unauthorized")
}

func (s *Server) guard(mode authMode, r *http.Request) reply {
	if mode == authNone {
		return reply{}
	}
	if mode == authPlugins && r.Header.Get("X-API-Key") != "" {
		return unauthorized()
	}
	if !s.tokens.access[bearerToken(r)] {
		return unauthorized()
	}
	return reply{}
}

func (s *Server) authRoutes() {
	s.route("GET /api/config", authNone, s.getConfig)
	s.route("GET /api/setup/status", authNone, s.setupStatus)
	s.route("POST /api/setup", authNone, s.setup)
	s.route("POST /api/auth/login", authNone, s.login)
	s.route("POST /api/auth/refresh", authNone, s.refresh)
	s.route("GET /api/me", authUser, s.me)
	s.route("POST /api/me/password", authUser, s.changePassword)
}

func (s *Server) getConfig(*request) reply {
	language := s.st.Language
	if language == "" {
		language = "en"
	}
	engines := s.st.EnabledEngines
	if engines == nil {
		engines = []string{}
	}
	return ok(api.Config{Language: language, EnabledEngines: engines})
}

func (s *Server) setupStatus(*request) reply {
	return ok(api.SetupStatus{Initialized: s.st.Initialized})
}

func (s *Server) setup(q *request) reply {
	if s.st.Initialized {
		return errReply(http.StatusConflict, "system already initialized")
	}
	var req api.SetupRequest
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if req.Username == "" {
		return errReply(http.StatusBadRequest, validatorMessage("setupRequest", "Username", "required"))
	}
	if len([]rune(req.Password)) < minPasswordLength {
		return errReply(http.StatusBadRequest, validatorMessage("setupRequest", "Password", "min"))
	}
	s.st.Initialized = true
	s.st.Username = req.Username
	s.st.Password = req.Password
	s.account = account{displayName: req.DisplayName, createdAt: s.now()}
	return ok(s.issuePair())
}

func (s *Server) login(q *request) reply {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if _, good := q.decode(&req); !good {
		return errReply(http.StatusBadRequest, "invalid request body")
	}
	if !s.st.Initialized || req.Username != s.st.Username || req.Password != s.st.Password {
		return errReply(http.StatusUnauthorized, "invalid username or password")
	}
	return ok(s.issuePair())
}

func (s *Server) refresh(q *request) reply {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if _, good := q.decode(&req); !good {
		return errReply(http.StatusBadRequest, "invalid request body")
	}
	if !s.tokens.refresh[req.RefreshToken] {
		return errReply(http.StatusUnauthorized, "invalid or expired refresh token")
	}
	return ok(map[string]any{
		"access_token": s.issueAccess(),
		"expires_in":   int64(accessTTL / time.Second),
	})
}

func (s *Server) me(*request) reply {
	return ok(map[string]any{
		"id":           userID,
		"username":     s.st.Username,
		"display_name": s.account.displayName,
		"created_at":   s.account.createdAt,
		"updated_at":   s.account.createdAt,
	})
}

func (s *Server) changePassword(q *request) reply {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if len([]rune(req.NewPassword)) < minPasswordLength {
		return errReply(http.StatusBadRequest, validatorMessage("changePasswordRequest", "NewPassword", "min"))
	}
	if req.OldPassword != s.st.Password {
		return codedReply(http.StatusBadRequest, "old_password_incorrect", "old password is incorrect", nil)
	}
	s.st.Password = req.NewPassword
	clear(s.tokens.access)
	clear(s.tokens.refresh)
	return noContent()
}
