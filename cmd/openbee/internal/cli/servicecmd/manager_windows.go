//go:build windows

package servicecmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

//go:embed templates/schtask.xml.tmpl
var windowsTemplatesFS embed.FS

const schtaskName = "OpenBee"

var schtaskTmpl = parseTemplate(windowsTemplatesFS, "templates/schtask.xml.tmpl", "schtask")

type schtaskTemplateData struct {
	UserId     string
	ExePath    string
	ConfigPath string
	LogPath    string
}

func renderSchtaskXML(d schtaskTemplateData) (string, error) {
	return executeTemplate(schtaskTmpl, d)
}

type windowsManager struct{}

func NewManager() (Manager, error) { return windowsManager{}, nil }

func currentUsername() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func (windowsManager) Install(ctx context.Context, opts InstallOptions) error {
	username, err := currentUsername()
	if err != nil {
		return err
	}
	xml, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     username,
		ExePath:    opts.ExePath,
		ConfigPath: opts.ConfigPath,
		LogPath:    opts.LogPath,
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "openbee-schtask-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encodeUTF16LE(xml)); err != nil {
		return err
	}
	tmp.Close()

	args := []string{"/Create", "/XML", tmp.Name(), "/TN", schtaskName}
	if opts.Force {
		args = append(args, "/F")
	}
	out, err := runCommand(ctx, "schtasks", args...)
	if err != nil {
		if !opts.Force && taskAlreadyExists(out) {
			return errors.New(i18n.M.Output.Service.AlreadyInstalled)
		}
		return wrapRunErr("schtasks /Create", err, out)
	}
	if !opts.AutoStart {
		return nil
	}
	if _, err := runOrWrap(ctx, "schtasks /Run", "schtasks", "/Run", "/TN", schtaskName); err != nil {
		return err
	}
	return nil
}

func (windowsManager) Uninstall(ctx context.Context) error {
	_, _ = runCommand(ctx, "schtasks", "/End", "/TN", schtaskName)
	out, err := runCommand(ctx, "schtasks", "/Delete", "/TN", schtaskName, "/F")
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "cannot find") {
			return nil
		}
		return wrapRunErr("schtasks /Delete", err, out)
	}
	return nil
}

func (windowsManager) Start(ctx context.Context) error {
	_, err := runOrWrap(ctx, "schtasks /Run", "schtasks", "/Run", "/TN", schtaskName)
	return err
}

func (windowsManager) Stop(ctx context.Context) error {
	_, err := runOrWrap(ctx, "schtasks /End", "schtasks", "/End", "/TN", schtaskName)
	return err
}

func (windowsManager) Status(ctx context.Context) (Status, error) {
	st := Status{}
	out, err := runCommand(ctx, "schtasks", "/Query", "/TN", schtaskName, "/V", "/FO", "LIST")
	if err != nil {
		return st, nil
	}
	st.Installed = true
	switch strings.TrimSpace(parseSchtasksField(string(out), "Status:")) {
	case "Running":
		st.RunState = RunStateRunning
	case "Ready":
		st.RunState = RunStateStopped
	}
	if st.RunState == RunStateRunning {
		if pid := lookupOpenbeePID(ctx); pid > 0 {
			st.PID = pid
		}
	}
	return st, nil
}

func taskAlreadyExists(out []byte) bool {
	lower := strings.ToLower(string(out))
	return strings.Contains(lower, "already exists")
}

func parseSchtasksField(s, key string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, key); i >= 0 {
			return strings.TrimSpace(line[i+len(key):])
		}
	}
	return ""
}

func lookupOpenbeePID(ctx context.Context) int {
	out, err := runCommand(ctx, "tasklist", "/FI", "IMAGENAME eq openbee.exe", "/FO", "CSV", "/NH")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			pidStr := strings.Trim(fields[1], `" `)
			if pid, err := strconv.Atoi(pidStr); err == nil {
				return pid
			}
		}
	}
	return 0
}

func encodeUTF16LE(s string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE})
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			hi := uint16(0xD800 + (r >> 10))
			lo := uint16(0xDC00 + (r & 0x3FF))
			buf.WriteByte(byte(hi))
			buf.WriteByte(byte(hi >> 8))
			buf.WriteByte(byte(lo))
			buf.WriteByte(byte(lo >> 8))
		} else {
			buf.WriteByte(byte(r))
			buf.WriteByte(byte(r >> 8))
		}
	}
	return buf.Bytes()
}
