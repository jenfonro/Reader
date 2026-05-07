package static

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jenfonro/reader/public"
)

var dist fs.FS

func init() {
	sub, err := fs.Sub(public.Dist, "dist")
	if err == nil {
		dist = sub
	}
}

func Handler() http.Handler {
	indexHTML := patchIndexHTML(mustReadFile("index.html"))
	fsHandler := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dist == nil {
			http.Error(w, "dist not found", http.StatusInternalServerError)
			return
		}

		if r.URL.Path == "/" {
			writeIndex(w, indexHTML)
			return
		}

		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "." || clean == "" {
			writeIndex(w, indexHTML)
			return
		}

		if f, err := dist.Open(clean); err == nil {
			_ = f.Close()
			fsHandler.ServeHTTP(w, r)
			return
		}

		if path.Ext(clean) == "" {
			writeIndex(w, indexHTML)
			return
		}
		http.NotFound(w, r)
	})
}

func writeIndex(w http.ResponseWriter, html string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, html)
}

func patchIndexHTML(html string) string {
	version := strings.TrimSpace(os.Getenv("ASSET_VERSION"))
	if version == "" {
		version = strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	}
	html = strings.ReplaceAll(html, "__ASSET_VERSION__", version)
	html = strings.ReplaceAll(html, "__READER_VERSION__", version)
	return html
}

func mustReadFile(name string) string {
	if dist == nil {
		return ""
	}
	f, err := dist.Open(name)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(b)
}
