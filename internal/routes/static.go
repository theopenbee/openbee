package routes

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerStaticRoutes() error {
	sub, err := fs.Sub(s.StaticFS, "dist")
	if err != nil {
		return fmt.Errorf("static assets: %w", err)
	}
	httpFS := http.FS(sub)

	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return fmt.Errorf("reading index.html: %w", err)
	}

	s.router.NoRoute(func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path != "" {
			f, err := sub.Open(path)
			if err == nil {
				f.Close()
				c.FileFromFS(path, httpFS)
				return
			}
		}
		// Serve index.html directly — must NOT use c.FileFromFS("index.html", ...)
		// because http.FileServer redirects any URL ending in /index.html to ./,
		// causing an infinite redirect loop.
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
	return nil
}
