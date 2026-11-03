package snap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

func TestSnapModuleContract(t *testing.T) {
	m := New()

	if m.ID() != ModuleID {
		t.Errorf("expected ID %q, got %q", ModuleID, m.ID())
	}
	if m.Name() != ModuleName {
		t.Errorf("expected Name %q, got %q", ModuleName, m.Name())
	}
	if m.Category() != ModuleCategory {
		t.Errorf("expected Category %q, got %q", ModuleCategory, m.Category())
	}
	if !m.RequiresElevation() {
		t.Error("expected RequiresElevation() to be true")
	}

	// Check IsAvailable matches host runtime
	if distro.HasSnap() {
		if !m.IsAvailable() {
			t.Error("expected IsAvailable() to be true when snap is installed")
		}
	} else {
		if m.IsAvailable() {
			t.Error("expected IsAvailable() to be false when snap is not installed")
		}
	}
}

func TestParseSnapListOutput(t *testing.T) {
	rawOutput := `Name               Version          Rev    Tracking       Publisher   Notes
canonical-livepatch 10.7.0           264    latest/stable  canonical✓  -
canonical-livepatch 10.6.0           258    latest/stable  canonical✓  disabled
core18             20231027         2812   latest/stable  canonical✓  base
core18             20230919         2796   latest/stable  canonical✓  base,disabled
core20             20240111         2182   latest/stable  canonical✓  base
core20             20231123         2105   latest/stable  canonical✓  base,disabled
firefox            122.0-1          3779   latest/stable/… mozilla✓    -
firefox            121.0.1-1        3720   latest/stable/… mozilla✓    disabled
`

	revs := ParseSnapListOutput(rawOutput)

	if len(revs) != 8 {
		t.Fatalf("expected 8 revisions parsed, got %d", len(revs))
	}

	var disabledCount int
	for _, r := range revs {
		if r.Disabled {
			disabledCount++
		}
	}

	if disabledCount != 4 {
		t.Fatalf("expected 4 disabled revisions, got %d", disabledCount)
	}

	if revs[1].Name != "canonical-livepatch" || revs[1].Revision != "258" || !revs[1].Disabled {
		t.Errorf("unexpected rev 1: %+v", revs[1])
	}
	if revs[3].Name != "core18" || revs[3].Revision != "2796" || !revs[3].Disabled {
		t.Errorf("unexpected rev 3: %+v", revs[3])
	}
	if revs[7].Name != "firefox" || revs[7].Revision != "3720" || !revs[7].Disabled {
		t.Errorf("unexpected rev 7: %+v", revs[7])
	}
}

func TestSnapScanAndDryRun(t *testing.T) {
	defer distro.ResetLookPathMock()

	distro.SetLookPathMock(func(binary string) (string, error) {
		if binary == "snap" {
			return "/usr/bin/snap", nil
		}
		return "", os.ErrNotExist
	})

	m := New()
	if !m.IsAvailable() {
		t.Fatal("expected IsAvailable() to be true when mocked")
	}

	tmpSnapsDir := t.TempDir()
	m.SetSnapsDir(tmpSnapsDir)

	// Create dummy .snap files for disabled revisions
	snap1 := filepath.Join(tmpSnapsDir, "core18_2796.snap")
	snap2 := filepath.Join(tmpSnapsDir, "firefox_3720.snap")
	_ = os.WriteFile(snap1, []byte("core18-snap-data-bytes"), 0644)
	_ = os.WriteFile(snap2, []byte("firefox-snap-data-payload-bytes"), 0644)

	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "snap" && len(args) == 2 && args[0] == "list" && args[1] == "--all" {
			return []byte(`Name               Version          Rev    Tracking       Publisher   Notes
core18             20231027         2812   latest/stable  canonical✓  base
core18             20230919         2796   latest/stable  canonical✓  base,disabled
firefox            122.0-1          3779   latest/stable  mozilla✓    -
firefox            121.0.1-1        3720   latest/stable  mozilla✓    disabled
`), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	})

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	if len(res.Items) != 2 {
		t.Fatalf("expected 2 disabled items, got %d", len(res.Items))
	}

	expectedSize1 := int64(len("core18-snap-data-bytes"))
	expectedSize2 := int64(len("firefox-snap-data-payload-bytes"))
	expectedTotal := expectedSize1 + expectedSize2

	if res.TotalSize != expectedTotal {
		t.Errorf("expected total size %d, got %d", expectedTotal, res.TotalSize)
	}

	// Test Clean with dry-run
	cleanRes, err := m.Clean(context.Background(), res.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	if cleanRes.FreedBytes != expectedTotal {
		t.Errorf("expected %d freed bytes in dry-run, got %d", expectedTotal, cleanRes.FreedBytes)
	}

	// Ensure files were not deleted during dry run
	if _, err := os.Stat(snap1); os.IsNotExist(err) {
		t.Error("snap file was deleted during dry run!")
	}
}

func TestSnapCleanExecution(t *testing.T) {
	tmpSnapsDir := t.TempDir()
	m := New()
	m.SetSnapsDir(tmpSnapsDir)

	snap1 := filepath.Join(tmpSnapsDir, "core18_2796.snap")
	_ = os.WriteFile(snap1, []byte("data"), 0644)

	var executedCommands [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmd)
		return []byte("done"), nil
	})

	items := []core.CleanableItem{
		{
			ID:       "snap:core18:2796",
			Path:     snap1,
			Size:     4,
			Metadata: map[string]string{"name": "core18", "revision": "2796", "version": "20230919"},
		},
	}

	cleanRes, err := m.Clean(context.Background(), items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	if cleanRes.FreedBytes != 4 {
		t.Errorf("expected 4 freed bytes, got %d", cleanRes.FreedBytes)
	}

	// Verify parameterized execution: snap remove core18 --revision=2796
	expectedCalls := [][]string{
		{"snap", "remove", "core18", "--revision=2796"},
	}

	if len(executedCommands) != len(expectedCalls) {
		t.Fatalf("expected %d commands executed, got %d: %v", len(expectedCalls), len(executedCommands), executedCommands)
	}

	gotStr := strings.Join(executedCommands[0], " ")
	expStr := strings.Join(expectedCalls[0], " ")
	if gotStr != expStr {
		t.Errorf("mismatch: got %q, want %q", gotStr, expStr)
	}

	// Ensure .snap file was cleaned from test tmpSnapsDir
	if _, err := os.Stat(snap1); !os.IsNotExist(err) {
		t.Error("expected snap1 to be removed from test snapsDir")
	}
}

func TestSnapContextCancellation(t *testing.T) {
	m := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items := []core.CleanableItem{
		{ID: "snap:test:1", Metadata: map[string]string{"name": "test", "revision": "1"}},
	}

	_, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}
