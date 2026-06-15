//go:build linux

package servicecmd

import (
	"context"
	"embed"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func init() {
	lookupRunAsEnvPath = linuxLookupRunAsEnvPath
	verifyNodeForRunAsUser = linuxVerifyNodeForRunAsUser
}

// linuxLookupRunAsEnvPath returns the login-shell PATH for username. We need
// the *daemon's* PATH baked into the unit, not the installer's: when sudo runs
// install, os.Getenv("PATH") is root's PATH (or sudo's secure_path), which
// often points at /root/.nvm/.../bin/node — a node binary the daemon user
// cannot exec, surfacing as "/usr/bin/env: 'node': Permission denied".
//
// `runuser -l <user> -c 'printf %s "$PATH"'` runs as preflightRoot guarantees
// root. `-l` is critical: without it runuser preserves the caller's env, so
// root's PATH (e.g. /root/.nvm/...) leaks into what we think is the run-as
// user's PATH and ends up baked into the unit — a path the daemon user can't
// even read. `-l` clears env and starts a real login shell so we get the
// run-as user's actual profile-derived PATH.
func linuxLookupRunAsEnvPath(ctx context.Context, username string) (string, error) {
	if username == "" {
		return "", nil
	}
	out, err := runCommand(ctx, "runuser", "-l", username, "-c", `printf %s "$PATH"`)
	if err != nil {
		return "", wrapRunErr("runuser", []string{"-u", username}, err, out)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", errors.New("runuser returned an empty PATH")
	}
	return p, nil
}

// linuxVerifyNodeForRunAsUser distinguishes the two failure modes the daemon
// will see at runtime:
//   - node not on PATH at all       → env reports "No such file or directory"
//   - node on PATH but not exec'able → env reports "Permission denied"
//
// We run the same lookup dance `env` does, as the run-as user with the exact
// PATH the unit will embed, so a green check here means chat will succeed.
func linuxVerifyNodeForRunAsUser(ctx context.Context, username, envPath string) nodeCheckResult {
	if username == "" {
		return nodeCheckUnknown
	}
	// Inside the shell: command -v finds node via PATH, then test -x verifies
	// the resolved binary is executable. Two distinct exits let us tell the
	// modes apart: 1 → missing, 2 → found-but-not-executable.
	script := `p="$(command -v node 2>/dev/null)"; [ -n "$p" ] || exit 1; [ -x "$p" ] || exit 2; exit 0`
	code, err := runWithExitCode(ctx, "runuser", "-u", username, "--", "/usr/bin/env", "-i", "PATH="+envPath, "/bin/sh", "-c", script)
	if err != nil {
		return nodeCheckUnknown
	}
	switch code {
	case 0:
		return nodeCheckOK
	case 1:
		return nodeCheckMissing
	case 2:
		return nodeCheckNotExecutable
	default:
		return nodeCheckUnknown
	}
}

//go:embed templates/systemd.service.tmpl
var linuxTemplatesFS embed.FS

const systemdUnitName = "openbee.service"

// systemdUnitDir is the directory the unit is written to. Overridden in tests.
var systemdUnitDir = "/etc/systemd/system"

// euid is overridden in tests so we can exercise the root preflight without
// actually being root.
var euid = os.Geteuid

var systemdTmpl = parseTemplate(linuxTemplatesFS, "templates/systemd.service.tmpl", "systemd")

type systemdTemplateData struct {
	ExePath    string
	ConfigPath string
	LogPath    string
	WorkingDir string
	Home       string
	EnvPath    string
	RunAsUser  string
	RunAsGroup string
}

func renderSystemdUnit(d systemdTemplateData) (string, error) {
	return executeTemplate(systemdTmpl, d)
}

type linuxManager struct{}

func NewManager() (Manager, error) {
	if _, err := execLookPath("systemctl"); err != nil {
		return nil, errors.New(i18n.M.Output.Service.SystemdUnavail)
	}
	return linuxManager{}, nil
}

func (linuxManager) unitPath() string {
	return filepath.Join(systemdUnitDir, systemdUnitName)
}

// preflightRoot guards every state-changing entry point. The system-wide unit
// directory and `systemctl daemon-reload` both require root; failing here lets
// the user see one actionable error instead of a partial half-installed state.
func preflightRoot() error {
	if euid() != 0 {
		return errors.New(i18n.M.Output.Service.MustBeRoot)
	}
	return nil
}

func (m linuxManager) Install(ctx context.Context, opts InstallOptions) error {
	if err := preflightRoot(); err != nil {
		return err
	}
	up := m.unitPath()
	_, statErr := os.Stat(up)
	existed := statErr == nil
	if existed && !opts.Force {
		return errors.New(i18n.M.Output.Service.AlreadyInstalled)
	}
	if err := os.MkdirAll(filepath.Dir(up), 0o755); err != nil {
		return err
	}
	unit, err := renderSystemdUnit(systemdTemplateData{
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
		WorkingDir: opts.WorkingDir,
		Home:       opts.Home,
		EnvPath:    opts.EnvPath,
		RunAsUser:  opts.RunAsUser,
		RunAsGroup: opts.RunAsGroup,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(up, []byte(unit), 0o644); err != nil {
		return err
	}
	// Make WorkingDir owned by RunAsUser so the daemon (which drops to that
	// user) can write logs into it even when the directory was just created by
	// root inside resolveInstallOptions.
	if err := chownWorkingDir(opts); err != nil {
		_ = os.Remove(up)
		return err
	}
	// Roll back the unit on systemctl failure so a retry won't hit the
	// already-installed guard. Skip on Force overwrite (caller accepted the
	// loss of the prior unit) — there's no "prior state" to restore.
	rollback := func(opErr error) error {
		if !existed {
			_ = os.Remove(up)
		}
		return opErr
	}
	if _, err := runOrWrap(ctx, "systemctl", "daemon-reload"); err != nil {
		return rollback(err)
	}
	enableArgs := []string{"enable", systemdUnitName}
	if opts.AutoStart {
		enableArgs = []string{"enable", "--now", systemdUnitName}
	}
	if _, err := runOrWrap(ctx, "systemctl", enableArgs...); err != nil {
		return rollback(err)
	}
	return nil
}

// chownWorkingDir is a no-op if RunAsUser is empty (tests) or the lookup
// fails. We deliberately do not recurse — the unit only writes logs at the
// directory root, and a recursive chown could clobber permissions on user
// data the operator already placed there.
var chownWorkingDir = func(opts InstallOptions) error {
	if opts.RunAsUser == "" || opts.WorkingDir == "" {
		return nil
	}
	uid, gid, err := lookupUIDGID(opts.RunAsUser, opts.RunAsGroup)
	if err != nil {
		return nil
	}
	return os.Chown(opts.WorkingDir, uid, gid)
}

func (m linuxManager) Uninstall(ctx context.Context) error {
	if err := preflightRoot(); err != nil {
		return err
	}
	up := m.unitPath()
	_, _ = runCommand(ctx, "systemctl", "disable", "--now", systemdUnitName)
	if err := os.Remove(up); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = runCommand(ctx, "systemctl", "daemon-reload")
	return nil
}

func (linuxManager) Start(ctx context.Context) error {
	if err := preflightRoot(); err != nil {
		return err
	}
	_, err := runOrWrap(ctx, "systemctl", "start", systemdUnitName)
	return err
}

func (linuxManager) Stop(ctx context.Context) error {
	if err := preflightRoot(); err != nil {
		return err
	}
	_, err := runOrWrap(ctx, "systemctl", "stop", systemdUnitName)
	return err
}

func (m linuxManager) Status(ctx context.Context) (Status, error) {
	up := m.unitPath()
	st := Status{}
	if _, err := os.Stat(up); err == nil {
		st.Installed = true
	}
	out, err := runCommand(ctx, "systemctl", "show", "-p", "ActiveState,SubState,MainPID,ExecMainStartTimestamp", systemdUnitName)
	if err != nil {
		if !st.Installed {
			return st, nil
		}
		st.RunState = RunStateStopped
		return st, nil
	}
	props := parseSystemctlShow(string(out))
	switch props["ActiveState"] {
	case "active":
		st.RunState = RunStateRunning
	case "failed":
		st.RunState = RunStateFailed
	default:
		st.RunState = RunStateStopped
	}
	if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
		st.PID = pid
	}
	return st, nil
}

func lookupUIDGID(username, groupname string) (int, int, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gidStr := u.Gid
	if groupname != "" {
		if g, err := user.LookupGroup(groupname); err == nil {
			gidStr = g.Gid
		}
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func parseSystemctlShow(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "="); i > 0 {
			out[line[:i]] = line[i+1:]
		}
	}
	return out
}
