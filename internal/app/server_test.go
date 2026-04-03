package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type logPayload struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

func TestLogsDeleteAndExport(t *testing.T) {
	t.Parallel()

	s := NewServer(t.TempDir())
	h := s.APIHandler()

	clearReq := httptest.NewRequest(http.MethodDelete, "/api/logs", nil)
	clearResp := httptest.NewRecorder()
	h.ServeHTTP(clearResp, clearReq)

	if clearResp.Code != http.StatusOK {
		t.Fatalf("unexpected clear status: %d", clearResp.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	listResp := httptest.NewRecorder()
	h.ServeHTTP(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("unexpected list status: %d", listResp.Code)
	}

	var logs []logPayload
	if err := json.Unmarshal(listResp.Body.Bytes(), &logs); err != nil {
		t.Fatalf("decode logs payload: %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "log buffer cleared" {
		t.Fatalf("unexpected logs after clear: %#v", logs)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/logs/export", nil)
	exportResp := httptest.NewRecorder()
	h.ServeHTTP(exportResp, exportReq)

	if exportResp.Code != http.StatusOK {
		t.Fatalf("unexpected export status: %d", exportResp.Code)
	}
	if got := exportResp.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if !strings.Contains(exportResp.Body.String(), "log buffer cleared") {
		t.Fatalf("export payload missing clear entry: %q", exportResp.Body.String())
	}
}

func TestLogsRejectInvalidSince(t *testing.T) {
	t.Parallel()

	s := NewServer(t.TempDir())
	h := s.APIHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/logs?since=12a", nil)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status for invalid since: %d", resp.Code)
	}
}

func TestLogsRejectUnsupportedMethod(t *testing.T) {
	t.Parallel()

	s := NewServer(t.TempDir())
	h := s.APIHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/logs", nil)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status for POST /api/logs: %d", resp.Code)
	}
}
