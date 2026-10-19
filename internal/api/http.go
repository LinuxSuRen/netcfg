package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

// statusByCode maps stable error codes to HTTP statuses.
func statusByCode(code string) int {
	switch code {
	case netconf.CodeInvalidArgument:
		return http.StatusBadRequest
	case netconf.CodeNotFound:
		return http.StatusNotFound
	case netconf.CodePermissionDenied:
		return http.StatusForbidden
	case netconf.CodeUnsupported:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, err error) {
	code := netconf.CodeOf(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusByCode(code))
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": err.Error()},
	})
}

func decodeBody(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(target); err != nil {
		return &netconf.Error{Code: netconf.CodeInvalidArgument,
			Err: fmt.Errorf("invalid JSON body: %v", err)}
	}
	return nil
}

// spaHandler serves an embedded SPA: real files directly, anything else
// falls back to index.html so client routes survive refreshes.
func spaHandler(webFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean(r.URL.Path)
		if name == "/" || name == "." {
			name = "index.html"
		}
		name = strings.TrimPrefix(name, "/")
		data, err := fs.ReadFile(webFS, name)
		if err != nil {
			if name == "index.html" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, "<h1>netcfg</h1><p>web UI not built; run <code>make build-web</code> or use the release binary.</p>")
				return
			}
			// SPA fallback for client-side routes.
			if data, err = fs.ReadFile(webFS, "index.html"); err != nil {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		switch {
		case strings.HasSuffix(name, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(name, ".js"):
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case strings.HasSuffix(name, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(name, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
	})
}
