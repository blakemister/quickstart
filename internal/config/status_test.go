package config

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// statusProbeTimeoutForTest sets the probe timeout to d and returns a restore
// func that reverts it.
func statusProbeTimeoutForTest(d time.Duration) (restore func()) {
	prev := statusProbeTimeout
	statusProbeTimeout = d
	return func() { statusProbeTimeout = prev }
}

// osPath returns the host PATH so a probe can resolve real executables (e.g.
// ping) in tests that intentionally exercise the timeout path.
func osPath() string { return os.Getenv("PATH") }

// exitCmd returns a shell command string that exits with the given code,
// portable across the host the tests run on.
func exitCmd(code string) string {
	if runtime.GOOS == "windows" {
		return "cmd /c exit " + code
	}
	return "sh -c exit_" + code // unreachable on non-windows in practice
}

func TestExpectedKeysPresent(t *testing.T) {
	env := []string{"A=1", "B=2", "EMPTY=", "c=3"}
	tests := []struct {
		name      string
		expected  []string
		wantHave  int
		wantTotal int
	}{
		{"all present", []string{"A", "B"}, 2, 2},
		{"case-insensitive", []string{"C"}, 1, 1},
		{"empty counts as missing", []string{"EMPTY"}, 0, 1},
		{"partial", []string{"A", "MISSING", "B"}, 2, 3},
		{"none", nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			have, total := ExpectedKeysPresent(env, tt.expected)
			if have != tt.wantHave || total != tt.wantTotal {
				t.Errorf("ExpectedKeysPresent = (%d, %d), want (%d, %d)", have, total, tt.wantHave, tt.wantTotal)
			}
		})
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"simple", "claude auth status", []string{"claude", "auth", "status"}},
		{
			"quoted path and arg with spaces",
			`"C:/Program Files/x/p.exe" --flag "a b"`,
			[]string{"C:/Program Files/x/p.exe", "--flag", "a b"},
		},
		{"collapses runs of whitespace", "a   b\t c", []string{"a", "b", "c"}},
		{"empty quoted token", `cmd ""`, []string{"cmd", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCommand(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCommand(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitCommand(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestProbeServiceStatusTimesOut verifies a hung StatusCmd is reported as a
// timeout error rather than wedging the probe. It temporarily shortens the
// timeout so the test stays fast.
func TestProbeServiceStatusTimesOut(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("timeout stub uses Windows ping; project is Windows-only")
	}
	prev := statusProbeTimeoutForTest(100 * time.Millisecond)
	defer prev()

	// ping -n 30 sleeps ~30s; the 100ms timeout must fire first.
	svc := Service{ID: "slow", StatusCmd: "ping -n 30 127.0.0.1"}
	got := ProbeServiceStatus(svc, []string{"PATH=" + osPath()})
	if got.State != StatusError {
		t.Fatalf("state = %d, want StatusError", got.State)
	}
	if got.Detail != "probe timed out" {
		t.Fatalf("detail = %q, want \"probe timed out\"", got.Detail)
	}
}

func TestProbeServiceStatus(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("status command stubs use Windows cmd; project is Windows-only")
	}

	env := []string{"JIRA_TOKEN=abc"}

	tests := []struct {
		name string
		svc  Service
		env  []string
		want StatusState
	}{
		{
			name: "no env no cmd -> unknown",
			svc:  Service{ID: "bare"},
			env:  env,
			want: StatusUnknown,
		},
		{
			name: "required env missing -> unknown",
			svc:  Service{ID: "jira", RequiresEnv: []string{"JIRA_TOKEN", "JIRA_URL"}},
			env:  env,
			want: StatusUnknown,
		},
		{
			name: "required env present no cmd -> configured",
			svc:  Service{ID: "jira", RequiresEnv: []string{"JIRA_TOKEN"}},
			env:  env,
			want: StatusConfigured,
		},
		{
			name: "status cmd exits 0 -> reachable",
			svc:  Service{ID: "ok", StatusCmd: exitCmd("0")},
			env:  env,
			want: StatusReachable,
		},
		{
			name: "status cmd exits 1 -> error",
			svc:  Service{ID: "bad", StatusCmd: exitCmd("1")},
			env:  env,
			want: StatusError,
		},
		{
			name: "required env present and cmd 0 -> reachable",
			svc:  Service{ID: "full", RequiresEnv: []string{"JIRA_TOKEN"}, StatusCmd: exitCmd("0")},
			env:  env,
			want: StatusReachable,
		},
		{
			name: "required env missing skips cmd -> unknown",
			svc:  Service{ID: "skip", RequiresEnv: []string{"MISSING"}, StatusCmd: exitCmd("0")},
			env:  env,
			want: StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProbeServiceStatus(tt.svc, tt.env)
			if got.State != tt.want {
				t.Errorf("ProbeServiceStatus state = %d, want %d (detail: %q)", got.State, tt.want, got.Detail)
			}
			if got.ServiceID != tt.svc.ID {
				t.Errorf("expected ServiceID %q, got %q", tt.svc.ID, got.ServiceID)
			}
			if got.CheckedAt.IsZero() {
				t.Error("expected CheckedAt to be set")
			}
		})
	}
}
