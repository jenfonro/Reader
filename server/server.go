package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jenfonro/reader/internal/auth"
	"github.com/jenfonro/reader/internal/db"
	"github.com/jenfonro/reader/server/static"
)

type Config struct {
	Addr string
}

type Server struct {
	addr string
	db   *db.DB
	h    http.Handler
}

func New(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, errors.New("addr is required")
	}

	database, err := db.Open()
	if err != nil {
		return nil, err
	}
	authMw := auth.New(database)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/bootstrap", bootstrapHandler(database))
	mux.HandleFunc("/api/home", homeHandler)
	mux.HandleFunc("/api/login", loginHandler(authMw))
	mux.HandleFunc("/api/settings/system", systemSettingsHandler(database))
	mux.HandleFunc("/api/settings/users", usersHandler(database))
	mux.HandleFunc("/api/settings/users/", userHandler(database))
	mux.Handle("/", static.Handler())

	return &Server{
		addr: cfg.Addr,
		db:   database,
		h:    noStoreForAppShell(authMw.Middleware(mux)),
	}, nil
}

func (s *Server) Addr() string          { return s.addr }
func (s *Server) Handler() http.Handler { return s.h }
func (s *Server) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func noStoreForAppShell(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
