package httpapi

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
)

type runtimeSettingsStore interface {
	GetRuntimeSettings() (config.RuntimeSettings, error)
	UpdateRuntimeSettings(next config.RuntimeSettings) (config.RuntimeSettings, error)
}

type runtimeSettingsApplier func(next config.RuntimeSettings) error

type Server struct {
	scanner  *library.Scanner
	queue    *jobs.Queue
	settings runtimeSettingsStore
	apply    runtimeSettingsApplier

	uiEnabled   bool
	uiStaticDir string

	mux    *http.ServeMux
	server *http.Server
}

type Option func(*Server)

func WithUI(staticDir string, enabled bool) Option {
	return func(s *Server) {
		s.uiStaticDir = staticDir
		s.uiEnabled = enabled
	}
}

func WithRuntimeSettingsStore(store runtimeSettingsStore) Option {
	return func(s *Server) {
		s.settings = store
	}
}

func WithRuntimeSettingsApplier(apply runtimeSettingsApplier) Option {
	return func(s *Server) {
		s.apply = apply
	}
}

func NewServer(scanner *library.Scanner, queue *jobs.Queue, opts ...Option) *Server {
	s := &Server{
		scanner:   scanner,
		queue:     queue,
		uiEnabled: false,
		mux:       http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) ListenAndServe(addr string) error {
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/library/sources", s.handleListSources)
	s.mux.HandleFunc("/api/library/items", s.handleListItems)
	s.mux.HandleFunc("/api/library/items/", s.handleListEpisodesByItem)
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/jobs/stream", s.handleJobStream)
	s.mux.HandleFunc("/api/scan", s.handleScan)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if !s.uiEnabled || s.uiStaticDir == "" {
		http.NotFound(w, r)
		return
	}

	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	indexPath := filepath.Join(s.uiStaticDir, "index.html")

	if rel == "" || !strings.Contains(filepath.Base(rel), ".") {
		http.ServeFile(w, r, indexPath)
		return
	}

	filePath := filepath.Join(s.uiStaticDir, rel)
	if _, err := os.Stat(filePath); err != nil {
		// SPA fallback: non-existing static file path returns index
		http.ServeFile(w, r, indexPath)
		return
	}
	http.ServeFile(w, r, filePath)
}
