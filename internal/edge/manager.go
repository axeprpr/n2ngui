package edge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/axeprpr/n2nGUI/internal/config"
)

const maxLogEntries = 400

type LogEntry struct {
	ID        int64     `json:"id"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Status struct {
	Running       bool       `json:"running"`
	PID           int        `json:"pid"`
	BinaryPath    string     `json:"binaryPath"`
	BinaryFound   bool       `json:"binaryFound"`
	LastError     string     `json:"lastError"`
	LastStart     *time.Time `json:"lastStart,omitempty"`
	Platform      string     `json:"platform"`
	Arguments     []string   `json:"arguments"`
	ConfigPath    string     `json:"configPath"`
	LegacyINIPath string     `json:"legacyIniPath"`
}

type Manager struct {
	baseDir string

	mu        sync.Mutex
	cmd       *exec.Cmd
	logs      []LogEntry
	nextLogID int64
	lastError string
	lastStart *time.Time
	lastArgs  []string
}

func NewManager(baseDir string) *Manager {
	return &Manager{baseDir: baseDir}
}

func (m *Manager) Start(ctx context.Context, cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.mu.Unlock()
		return errors.New("edge process is already running")
	}
	m.mu.Unlock()

	binaryPath, found := FindBinary(m.baseDir)
	if !found {
		return fmt.Errorf("edge binary not found at %s", binaryPath)
	}

	args := cfg.EdgeArgs()
	cmd := exec.CommandContext(ctx, binaryPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		m.setLastError(err.Error())
		return err
	}

	startedAt := time.Now()

	m.mu.Lock()
	m.cmd = cmd
	m.lastStart = &startedAt
	m.lastArgs = append([]string(nil), args...)
	m.lastError = ""
	m.appendLogLocked("system", fmt.Sprintf("started edge process pid=%d", cmd.Process.Pid))
	m.mu.Unlock()

	go m.captureOutput("stdout", stdout)
	go m.captureOutput("stderr", stderr)
	go m.watchProcess(cmd)

	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return errors.New("edge process is not running")
	}

	if err := cmd.Process.Kill(); err != nil {
		m.setLastError(err.Error())
		return err
	}

	m.mu.Lock()
	m.appendLogLocked("system", "stop requested")
	m.mu.Unlock()
	return nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	binaryPath, found := FindBinary(m.baseDir)
	status := Status{
		BinaryPath:    binaryPath,
		BinaryFound:   found,
		LastError:     m.lastError,
		LastStart:     m.lastStart,
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		Arguments:     append([]string(nil), m.lastArgs...),
		ConfigPath:    config.FilePath(m.baseDir),
		LegacyINIPath: config.LegacyINIPath(m.baseDir),
	}

	if m.cmd != nil && m.cmd.Process != nil {
		status.Running = true
		status.PID = m.cmd.Process.Pid
	}

	return status
}

func (m *Manager) Logs(since int64) []LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	if since <= 0 {
		return append([]LogEntry(nil), m.logs...)
	}

	var out []LogEntry
	for _, entry := range m.logs {
		if entry.ID > since {
			out = append(out, entry)
		}
	}
	return out
}

func (m *Manager) ClearLogs() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logs = nil
	m.appendLogLocked("system", "log buffer cleared")
}

func (m *Manager) ExportLogs() string {
	m.mu.Lock()
	logs := append([]LogEntry(nil), m.logs...)
	m.mu.Unlock()

	var out strings.Builder
	for _, entry := range logs {
		out.WriteString(entry.Timestamp.Format(time.RFC3339))
		out.WriteString(" ")
		out.WriteString(strings.ToUpper(entry.Stream))
		out.WriteString(" ")
		out.WriteString(entry.Message)
		out.WriteByte('\n')
	}
	return out.String()
}

func (m *Manager) Diagnostics() map[string]any {
	status := m.Status()
	return map[string]any{
		"platform":      status.Platform,
		"binaryPath":    status.BinaryPath,
		"binaryFound":   status.BinaryFound,
		"configPath":    status.ConfigPath,
		"legacyIniPath": status.LegacyINIPath,
		"workingDir":    m.baseDir,
	}
}

func FindBinary(baseDir string) (string, bool) {
	name := "edge"
	if runtime.GOOS == "windows" {
		name = "edge.exe"
	}

	path := filepath.Join(baseDir, "n2n", name)
	_, err := os.Stat(path)
	return path, err == nil
}

func (m *Manager) captureOutput(stream string, input io.ReadCloser) {
	defer input.Close()

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		m.mu.Lock()
		m.appendLogLocked(stream, scanner.Text())
		m.mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		m.setLastError(err.Error())
	}
}

func (m *Manager) watchProcess(cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	exitMessage := "edge process exited"
	if err != nil {
		m.lastError = err.Error()
		exitMessage = fmt.Sprintf("edge process exited with error: %v", err)
	}
	m.appendLogLocked("system", exitMessage)

	if m.cmd == cmd {
		m.cmd = nil
	}
}

func (m *Manager) setLastError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = message
	m.appendLogLocked("system", message)
}

func (m *Manager) appendLogLocked(stream, message string) {
	m.nextLogID++
	m.logs = append(m.logs, LogEntry{
		ID:        m.nextLogID,
		Stream:    stream,
		Message:   message,
		Timestamp: time.Now(),
	})

	if len(m.logs) > maxLogEntries {
		m.logs = append([]LogEntry(nil), m.logs[len(m.logs)-maxLogEntries:]...)
	}
}
