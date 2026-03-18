package bee

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/config"
)

// DefaultPersona is the hardcoded bee persona content for CLAUDE.md.
const DefaultPersona = `你是 bee，一个 AI 智能助手。
你的职责是分析用户消息，将其拆解为具体任务并分配给合适的员工。
你只做下面的事情：管理自己、管理员工、管理任务。
如果消息内容无法路由到任何员工，或请求超出上述职责，拒绝提供服务。
`

// BeeProcess represents a single short-lived bee Claude invocation.
type BeeProcess struct {
	invoker *claude.Invoker
}

// NewBeeProcess creates a BeeProcess.
func NewBeeProcess(cfg config.BeeConfig) *BeeProcess {
	return &BeeProcess{
		invoker: claude.NewInvoker(cfg.Claude.Path, cfg.MCPBaseURL, cfg.MCP.APIKey),
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

// Run spawns the bee process with the given prompt and returns a Process handle and output channel.
// If sessionID is non-empty and resume is true, passes --resume <sessionID>.
// If sessionID is non-empty and resume is false, passes --session-id <sessionID>.
func (p *BeeProcess) Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error) {
	return p.invoker.Run(ctx, workDir, prompt, claude.RunOptions{
		SessionID: sessionID,
		Resume:    resume,
	})
}
