package edge

import (
	"strings"
	"testing"
	"time"
)

func TestClearLogsAndExport(t *testing.T) {
	t.Parallel()

	m := NewManager(t.TempDir())

	m.mu.Lock()
	m.logs = append(m.logs, LogEntry{
		ID:        1,
		Stream:    "stdout",
		Message:   "edge online",
		Timestamp: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	m.nextLogID = 1
	m.mu.Unlock()

	exported := m.ExportLogs()
	if !strings.Contains(exported, "STDOUT edge online") {
		t.Fatalf("unexpected export payload: %q", exported)
	}

	m.ClearLogs()
	logs := m.Logs(0)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log after clear, got %d", len(logs))
	}
	if logs[0].Message != "log buffer cleared" {
		t.Fatalf("unexpected clear message: %q", logs[0].Message)
	}
}
