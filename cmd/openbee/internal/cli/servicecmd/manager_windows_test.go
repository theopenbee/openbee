//go:build windows

package servicecmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRenderSchtaskXML(t *testing.T) {
	got, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     "DESKTOP-A\\me",
		ExePath:    `C:\Program Files\openbee\openbee.exe`,
		ConfigPath: `C:\Users\me\.openbee\config.yaml`,
		LogPath:    `C:\Users\me\.openbee\daemon.log`,
		WorkingDir: `C:\Users\me\.openbee`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<UserId>DESKTOP-A\\me</UserId>",
		"<RunLevel>LeastPrivilege</RunLevel>",
		"<Hidden>true</Hidden>",
		"<Interval>PT1M</Interval>",
		"openbee.exe",
		"server -c",
		"daemon.log",
		`<WorkingDirectory>C:\Users\me\.openbee</WorkingDirectory>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("XML missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestEncodeUTF16LE_HasBOM(t *testing.T) {
	got := encodeUTF16LE("A")
	if len(got) != 4 {
		t.Fatalf("want 4 bytes, got %d", len(got))
	}
	if got[0] != 0xFF || got[1] != 0xFE {
		t.Errorf("missing BOM: %x %x", got[0], got[1])
	}
	if got[2] != 'A' || got[3] != 0 {
		t.Errorf("wrong encoding: %x %x", got[2], got[3])
	}
}

// stubRunCommand wires runCommand to a per-command lookup table so each Status
// test can declare only the calls it cares about.
func stubRunCommand(t *testing.T, table map[string]func(args []string) ([]byte, error)) {
	t.Helper()
	prev := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		fn, ok := table[name]
		if !ok {
			return nil, errors.New("unexpected command: " + name)
		}
		return fn(args)
	}
	t.Cleanup(func() { runCommand = prev })
}

const fakeTasklistRow = `"openbee.exe","4242","Console","1","20,000 K"` + "\n"

// TestWindowsStatus_RunningViaPowerShell covers the happy path: Task Scheduler
// reports Running through Get-ScheduledTask (locale-independent), and the PID
// comes from tasklist.
func TestWindowsStatus_RunningViaPowerShell(t *testing.T) {
	stubRunCommand(t, map[string]func([]string) ([]byte, error){
		"schtasks":   func([]string) ([]byte, error) { return []byte("ok"), nil },
		"powershell": func([]string) ([]byte, error) { return []byte("Running\r\n"), nil },
		"tasklist":   func([]string) ([]byte, error) { return []byte(fakeTasklistRow), nil },
	})

	st, err := (windowsManager{}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed {
		t.Error("Installed = false, want true")
	}
	if st.RunState != RunStateRunning {
		t.Errorf("RunState = %v, want running", st.RunState)
	}
	if st.PID != 4242 {
		t.Errorf("PID = %d, want 4242", st.PID)
	}
}

// TestWindowsStatus_StoppedViaPowerShell asserts the "Ready" enum (idle task)
// maps to RunStateStopped — schtasks /Run will move it back to Running.
func TestWindowsStatus_StoppedViaPowerShell(t *testing.T) {
	stubRunCommand(t, map[string]func([]string) ([]byte, error){
		"schtasks":   func([]string) ([]byte, error) { return []byte("ok"), nil },
		"powershell": func([]string) ([]byte, error) { return []byte("Ready\r\n"), nil },
	})

	st, err := (windowsManager{}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.RunState != RunStateStopped {
		t.Errorf("RunState = %v, want stopped", st.RunState)
	}
}

// TestWindowsStatus_FallsBackToTasklistWhenPowerShellMissing is the regression
// guard for the non-English-locale bug: schtasks /Query /V /FO LIST output is
// localized (e.g. "状态: 正在运行" on Chinese Windows), so we deliberately do
// NOT parse it. Instead, when PowerShell is unreachable, we infer Running from
// a live openbee.exe in tasklist.
func TestWindowsStatus_FallsBackToTasklistWhenPowerShellMissing(t *testing.T) {
	stubRunCommand(t, map[string]func([]string) ([]byte, error){
		"schtasks":   func([]string) ([]byte, error) { return []byte("ok"), nil },
		"powershell": func([]string) ([]byte, error) { return nil, errors.New("powershell not found") },
		"tasklist":   func([]string) ([]byte, error) { return []byte(fakeTasklistRow), nil },
	})

	st, err := (windowsManager{}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed {
		t.Error("Installed = false, want true (schtasks /Query succeeded)")
	}
	if st.RunState != RunStateRunning {
		t.Errorf("RunState = %v, want running (via tasklist fallback)", st.RunState)
	}
	if st.PID != 4242 {
		t.Errorf("PID = %d, want 4242", st.PID)
	}
}

// TestWindowsStatus_NotInstalled exercises the schtasks-failure branch:
// missing task should leave Installed=false and RunState=unknown.
func TestWindowsStatus_NotInstalled(t *testing.T) {
	stubRunCommand(t, map[string]func([]string) ([]byte, error){
		"schtasks": func([]string) ([]byte, error) {
			return nil, errors.New("ERROR: The system cannot find the file specified.")
		},
	})

	st, err := (windowsManager{}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed {
		t.Error("Installed = true, want false when schtasks /Query fails")
	}
	if st.RunState != RunStateUnknown {
		t.Errorf("RunState = %v, want unknown", st.RunState)
	}
}
