package httpapi

import (
	"io"
	"net/http"
	"path"
)

// SPAHandler serves a static file or falls back to index.html for client routes.
type SPAHandler struct {
	root http.FileSystem
}

// NewSPAHandler creates a new SPA handler for the given root directory.
func NewSPAHandler(root string) *SPAHandler {
	return &SPAHandler{root: http.Dir(root)}
}

func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := path.Clean(r.URL.Path)
	if p == "." {
		p = "/"
	}

	// Try to open the requested file.
	f, err := h.root.Open(p)
	if err == nil {
		stat, err := f.Stat()
		if err == nil && !stat.IsDir() {
			// Real file; serve it.
			if path.Ext(p) == ".html" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeContent(w, r, p, stat.ModTime(), f.(io.ReadSeeker))
			return
		}
		f.Close()
	}

	// Fallback to index.html for client-side routing.
	index, err := h.root.Open("/index.html")
	if err != nil {
		RespondError(w, http.StatusServiceUnavailable, "前端未构建")
		return
	}
	defer index.Close()
	stat, err := index.Stat()
	if err != nil {
		RespondError(w, http.StatusServiceUnavailable, "前端未构建")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "/index.html", stat.ModTime(), index.(io.ReadSeeker))
}
