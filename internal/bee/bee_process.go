package bee

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/robobee/core/internal/config"
)

// DefaultPersona is the hardcoded bee persona content for CLAUDE.md.
const DefaultPersona = `你是 bee，一个 AI 智能助手。
你的职责是分析用户消息，将其拆解为具体任务并分配给合适的员工。
你只做下面的事情：管理自己、管理员工、管理任务。
如果消息内容无法路由到任何员工，或请求超出上述职责，拒绝提供服务。
`

// BeeProcess represents a single short-lived bee Claude invocation.
type BeeProcess struct {
	binary string
	mcpURL string
	apiKey string
}

// NewBeeProcess creates a BeeProcess.
func NewBeeProcess(cfg config.BeeConfig) *BeeProcess {
	return &BeeProcess{
		binary: cfg.Claude.Path,
		mcpURL: cfg.MCPBaseURL + "/mcp/sse",
		apiKey: cfg.MCP.APIKey,
	}
}

// WriteCLAUDEMD creates the CLAUDE.md file in workDir with persona content
// only if it does not already exist. This preserves any user edits.
func WriteCLAUDEMD(workDir, persona string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee workdir: %w", err)
	}
	path := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists, do not overwrite
	}
	return os.WriteFile(path, []byte(persona), 0o644)
}

// Run spawns the bee process with the given prompt and waits for it to exit.
// If sessionID is non-empty and resume is true, passes --resume <sessionID>.
// If sessionID is non-empty and resume is false, passes --session-id <sessionID>.
// Returns nil on exit code 0, an error otherwise.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) error {
	mcpConfig := fmt.Sprintf(
		`{"mcpServers":{"robobee":{"type":"sse","url":%q}}}`,
		p.mcpURL+"?api_key="+url.QueryEscape(p.apiKey),
	)
	args := []string{
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		"--mcp-config", mcpConfig,
		"-p", prompt,
	}
	if sessionID != "" {
		if resume {
			args = append(args, "--resume", sessionID)
		} else {
			args = append(args, "--session-id", sessionID)
		}
	}
	cmd := exec.CommandContext(ctx, p.binary, args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	// Create log file for this run.
	sid := sessionID
	if sid == "" {
		sid = "nosession"
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	logDir := filepath.Join(homeDir, ".robobee", "bee-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("mkdir bee-logs: %w", err)
	}
	logFileName := fmt.Sprintf("%s_%s.log", sid, time.Now().Format("20060102_150405"))
	logFile, err := os.OpenFile(filepath.Join(logDir, logFileName), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open bee log file: %w", err)
	}
	defer logFile.Close()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bee: %w", err)
	}

	// Drain stdout/stderr into log file.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Fprintf(logFile, "[stdout] %s\n", scanner.Text())
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintf(logFile, "[stderr] %s\n", scanner.Text())
		}
	}()

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("bee exited with error: %w", err)
	}
	return nil
}
