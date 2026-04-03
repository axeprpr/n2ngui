package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/axeprpr/n2nGUI/internal/config"
	"github.com/axeprpr/n2nGUI/internal/edge"
)

type Server struct {
	baseDir string
	manager *edge.Manager
}

func NewServer(baseDir string) *Server {
	return &Server{
		baseDir: baseDir,
		manager: edge.NewManager(baseDir),
	}
}

func (s *Server) APIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/control/start", s.handleStart)
	mux.HandleFunc("/api/control/stop", s.handleStop)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/logs/export", s.handleLogsExport)
	mux.HandleFunc("/api/diagnostics", s.handleDiagnostics)
	return withCORS(mux)
}

func (s *Server) Handler(staticFiles fs.FS) (http.Handler, error) {
	sub, err := fs.Sub(staticFiles, "frontend")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", s.APIHandler())

	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path == "/" {
			http.ServeFileFS(w, r, sub, "index.html")
			return
		}

		fileServer.ServeHTTP(w, r)
	})

	return withCORS(mux), nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.Load(s.baseDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		var cfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := config.Save(s.baseDir, cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cfg, err := config.Load(s.baseDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := s.manager.Start(ctx, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := s.manager.Stop(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.manager.ClearLogs()
		writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	since := int64(0)
	if value := strings.TrimSpace(r.URL.Query().Get("since")); value != "" {
		var parseErr error
		since, parseErr = parseInt64(value)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, parseErr)
			return
		}
	}

	writeJSON(w, http.StatusOK, s.manager.Logs(since))
}

func (s *Server) handleLogsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="n2n-edge.log"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.manager.ExportLogs()))
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	diagnostics := s.manager.Diagnostics()
	if _, err := os.Stat(config.FilePath(s.baseDir)); err == nil {
		diagnostics["configExists"] = true
	}
	if _, err := os.Stat(config.LegacyINIPath(s.baseDir)); err == nil {
		diagnostics["legacyIniExists"] = true
	}
	writeJSON(w, http.StatusOK, diagnostics)
}

func parseInt64(input string) (int64, error) {
	var value int64
	for _, r := range input {
		if r < '0' || r > '9' {
			return 0, errors.New("since must be a positive integer")
		}
		value = value*10 + int64(r-'0')
	}
	return value, nil
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
