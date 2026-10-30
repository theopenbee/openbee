package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const NodeCommand = "node"

type RuntimeKind string

const (
	RuntimeBun  RuntimeKind = "bun"
	RuntimeNode RuntimeKind = "node"
)

type Runtime struct {
	Kind RuntimeKind
	Exe  string
	Npm  string
	Env  []string
}

type Invocation struct {
	Path string
	Args []string
	Env  []string
}

type RuntimeResolver struct {
	Dir       string
	Mirror    string
	LookPath  func(string) (string, error)
	Client    *http.Client
	OnInstall func(version, dir string)

	mu sync.Mutex
}

func DefaultRuntimeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "runtime")
}

func NewRuntimeResolver() *RuntimeResolver {
	return &RuntimeResolver{Dir: DefaultRuntimeDir(), Mirror: os.Getenv("OPENBEE_BUN_MIRROR")}
}

func (r *RuntimeResolver) Resolve(ctx context.Context) (Runtime, error) {
	return r.resolve(ctx, false)
}

func (r *RuntimeResolver) ResolvePackageManager(ctx context.Context) (Runtime, error) {
	return r.resolve(ctx, true)
}

func (r *RuntimeResolver) resolve(ctx context.Context, needNpm bool) (Runtime, error) {
	if p, err := r.lookPath("bun"); err == nil {
		return Runtime{Kind: RuntimeBun, Exe: p}, nil
	}
	if rt, ok := r.localBun(); ok {
		return rt, nil
	}
	if node, err := r.lookPath("node"); err == nil {
		npm, err := r.lookPath("npm")
		if err == nil || !needNpm {
			return Runtime{Kind: RuntimeNode, Exe: node, Npm: npm}, nil
		}
	}
	return r.installLocalBun(ctx)
}

func (r *RuntimeResolver) Invocation(ctx context.Context, dir string, command []string) (Invocation, error) {
	if len(command) == 0 || command[0] == "" {
		return Invocation{}, errors.New("command is empty")
	}
	if r == nil || command[0] != NodeCommand {
		path, err := ResolveCommand(dir, command[0])
		if err != nil {
			return Invocation{}, err
		}
		return Invocation{Path: path, Args: command[1:]}, nil
	}
	rt, err := r.Resolve(ctx)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{Path: rt.Exe, Args: command[1:], Env: rt.Env}, nil
}

func (r *RuntimeResolver) lookPath(name string) (string, error) {
	if r.LookPath != nil {
		return r.LookPath(name)
	}
	return exec.LookPath(name)
}

func (r *RuntimeResolver) bunHome() string {
	return filepath.Join(r.Dir, "bun-"+BunVersion)
}

func (r *RuntimeResolver) bunExe() string {
	name := "bun"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(r.bunHome(), "bin", name)
}

func (r *RuntimeResolver) localBun() (Runtime, bool) {
	exe := r.bunExe()
	if info, err := os.Stat(exe); err != nil || info.IsDir() {
		return Runtime{}, false
	}
	home := r.bunHome()
	return Runtime{Kind: RuntimeBun, Exe: exe, Env: []string{
		"BUN_INSTALL=" + home,
		"BUN_INSTALL_CACHE_DIR=" + filepath.Join(home, "install", "cache"),
		"BUN_RUNTIME_TRANSPILER_CACHE_PATH=" + filepath.Join(home, "transpiler-cache"),
	}}, true
}

func (r *RuntimeResolver) installLocalBun(ctx context.Context) (Runtime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rt, ok := r.localBun(); ok {
		return rt, nil
	}
	target, err := bunTarget(runtime.GOOS, runtime.GOARCH, hasAVX2(), isMusl())
	if err != nil {
		return Runtime{}, fmt.Errorf("no usable bun or node found: %w", err)
	}
	if r.OnInstall != nil {
		r.OnInstall(BunVersion, r.bunHome())
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	mirrors := DefaultBunMirrors
	if r.Mirror != "" {
		mirrors = []string{r.Mirror}
	}
	err = installBun(ctx, client, mirrors, target, r.bunExe())
	if errors.Is(err, errBunUnusable) {
		return Runtime{}, fmt.Errorf("install Bun %s: %w", BunVersion, err)
	}
	if err != nil {
		return Runtime{}, fmt.Errorf("%w: %w", ErrBunDownload, err)
	}
	r.removeOtherBuns()
	rt, ok := r.localBun()
	if !ok {
		return Runtime{}, fmt.Errorf("%w: %s missing after install", ErrBunDownload, r.bunExe())
	}
	return rt, nil
}

func (r *RuntimeResolver) removeOtherBuns() {
	entries, _ := os.ReadDir(r.Dir)
	for _, e := range entries {
		dir := filepath.Join(r.Dir, e.Name())
		if e.IsDir() && strings.HasPrefix(e.Name(), "bun-") && dir != r.bunHome() {
			os.RemoveAll(dir)
		}
	}
}
