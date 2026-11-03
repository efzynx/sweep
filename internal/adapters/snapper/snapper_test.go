package snapper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

func TestSnapperModule_Metadata(t *testing.T) {
	m := New()

	if m.ID() != "snapper" {
		t.Errorf("expected ID 'snapper', got %q", m.ID())
	}
	if m.Name() != "Btrfs & Snapper Snapshots" {
		t.Errorf("expected Name 'Btrfs & Snapper Snapshots', got %q", m.Name())
	}
	if m.Category() != "snapshots" {
		t.Errorf("expected Category 'snapshots', got %q", m.Category())
	}
	if !m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be true")
	}
	if m.Description() == "" {
		t.Errorf("expected non-empty description")
	}
}

func TestSnapperModule_IsAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	mountsFile := filepath.Join(tmpDir, "mounts")

	// 1. Mounts file with no btrfs
	_ = os.WriteFile(mountsFile, []byte("/dev/sda1 / ext4 rw,relatime 0 0\n"), 0644)
	m := New()
	m.SetMountsPath(mountsFile)

	if m.IsAvailable() {
		t.Errorf("expected IsAvailable to be false when no btrfs mounts exist")
	}

	// 2. Mounts file with btrfs
	_ = os.WriteFile(mountsFile, []byte("/dev/sda2 / btrfs rw,relatime 0 0\n"), 0644)
	if !m.hasBtrfsMounts() {
		t.Errorf("expected hasBtrfsMounts to be true")
	}
}

func TestSnapperModule_SafeguardSnapshotHook(t *testing.T) {
	m := New()
	var executedCmd []string

	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		executedCmd = append([]string{name}, args...)
		return []byte("Snapshot #50 created."), nil
	})

	err := m.CreateSafeguardSnapshot("Test safeguard snapshot")
	if err != nil {
		t.Fatalf("CreateSafeguardSnapshot failed: %v", err)
	}

	expected := []string{"snapper", "-c", "root", "create", "-d", "Test safeguard snapshot"}
	if len(executedCmd) != len(expected) {
		t.Fatalf("expected command length %d, got %d (%v)", len(expected), len(executedCmd), executedCmd)
	}
	for i := range expected {
		if executedCmd[i] != expected[i] {
			t.Errorf("arg %d: expected %s, got %s", i, expected[i], executedCmd[i])
		}
	}
}

func TestSnapperModule_ScanAndClassify(t *testing.T) {
	m := New()
	m.SetAvailable(true)
	m.SetConfigs([]string{"root"})

	now := time.Now()
	tRecent := now.Add(-1 * 24 * time.Hour).Format("2006-01-02 15:04:05")  // 1 day ago (<=3d)
	tMedium := now.Add(-5 * 24 * time.Hour).Format("2006-01-02 15:04:05")  // 5 days ago (>3d)
	tOlder := now.Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04:05")  // 10 days ago (>7d)
	tOldest := now.Add(-40 * 24 * time.Hour).Format("2006-01-02 15:04:05") // 40 days ago (>30d)

	csvOutput := fmt.Sprintf(`"number","default","active","date","type","description","used-space"
"0","yes","yes","","single","current subvolume","0 B"
"1","no","no","%s","single","daily backup","500 MiB"
"2","no","no","%s","timeline","timeline snapshot","200 MiB"
"3","no","no","%s","pre","pacman update pre","100 MiB"
"4","no","no","%s","post","pacman update post","150 MiB"
`, tOldest, tOlder, tMedium, tRecent)

	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdStr := strings.Join(args, " ")
		if strings.Contains(cmdStr, "list-configs") {
			return []byte("root\n"), nil
		}
		if strings.Contains(cmdStr, "list") {
			return []byte(csvOutput), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	})

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(res.Items) != 5 {
		t.Fatalf("expected 5 snapshot items, got %d", len(res.Items))
	}

	itemMap := make(map[string]core.CleanableItem)
	for _, it := range res.Items {
		itemMap[it.Metadata["snapshot_id"]] = it
	}

	// 1. Snapshot 0: MUST be protected, high safety, unselected
	s0, ok := itemMap["0"]
	if !ok {
		t.Errorf("snapshot 0 not found")
	} else {
		if s0.Selected {
			t.Errorf("snapshot 0 must NEVER be selected for deletion")
		}
		if s0.Safety != core.SafetyHigh {
			t.Errorf("snapshot 0 must have SafetyHigh, got %v", s0.Safety)
		}
		if s0.Metadata["protected"] != "true" {
			t.Errorf("snapshot 0 must be marked protected")
		}
	}

	// 2. Snapshot 1 (>30d): should be selected, SafetySafe
	s1, ok := itemMap["1"]
	if !ok {
		t.Errorf("snapshot 1 not found")
	} else {
		if !s1.Selected {
			t.Errorf("snapshot 1 (>30d) should be selected for deletion")
		}
		if s1.Safety != core.SafetySafe {
			t.Errorf("snapshot 1 should have SafetySafe, got %v", s1.Safety)
		}
		if s1.Metadata["age_category"] != ">30d" {
			t.Errorf("expected >30d age category, got %s", s1.Metadata["age_category"])
		}
		if s1.Size == 0 {
			t.Errorf("expected parsed size for snapshot 1")
		}
	}

	// 3. Snapshot 2 (>7d): unselected, SafetyCaution
	s2, ok := itemMap["2"]
	if !ok {
		t.Errorf("snapshot 2 not found")
	} else {
		if s2.Selected {
			t.Errorf("snapshot 2 should not be selected by default")
		}
		if s2.Safety != core.SafetyCaution {
			t.Errorf("snapshot 2 should have SafetyCaution, got %v", s2.Safety)
		}
		if s2.Metadata["age_category"] != ">7d" {
			t.Errorf("expected >7d age category, got %s", s2.Metadata["age_category"])
		}
	}

	// 4. Snapshot 3 (>3d): unselected, SafetyCaution
	s3, ok := itemMap["3"]
	if !ok {
		t.Errorf("snapshot 3 not found")
	} else {
		if s3.Selected {
			t.Errorf("snapshot 3 should not be selected by default")
		}
		if s3.Safety != core.SafetyCaution {
			t.Errorf("snapshot 3 should have SafetyCaution, got %v", s3.Safety)
		}
		if s3.Metadata["age_category"] != ">3d" {
			t.Errorf("expected >3d age category, got %s", s3.Metadata["age_category"])
		}
	}

	// 5. Snapshot 4 (<=3d): unselected, SafetyHigh
	s4, ok := itemMap["4"]
	if !ok {
		t.Errorf("snapshot 4 not found")
	} else {
		if s4.Selected {
			t.Errorf("snapshot 4 (<=3d) should not be selected")
		}
		if s4.Safety != core.SafetyHigh {
			t.Errorf("snapshot 4 should have SafetyHigh, got %v", s4.Safety)
		}
		if s4.Metadata["age_category"] != "<=3d" {
			t.Errorf("expected <=3d age category, got %s", s4.Metadata["age_category"])
		}
	}
}

func TestSnapperModule_CleanAndSafeguard(t *testing.T) {
	m := New()

	items := []core.CleanableItem{
		{
			ID:       "snapper:root:0",
			Size:     0,
			Selected: true,
			Metadata: map[string]string{
				"config":      "root",
				"snapshot_id": "0",
				"protected":   "true",
			},
		},
		{
			ID:       "snapper:root:10",
			Size:     1024 * 1024 * 50,
			Selected: true,
			Metadata: map[string]string{
				"config":      "root",
				"snapshot_id": "10",
			},
		},
		{
			ID:       "snapper:root:11",
			Size:     1024 * 1024 * 30,
			Selected: true,
			Metadata: map[string]string{
				"config":      "root",
				"snapshot_id": "11",
			},
		},
		{
			ID:       "snapper:home:5",
			Size:     1024 * 1024 * 20,
			Selected: true,
			Metadata: map[string]string{
				"config":      "home",
				"snapshot_id": "5",
			},
		},
	}

	var executedCmds [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdLine := append([]string{name}, args...)
		executedCmds = append(executedCmds, cmdLine)
		return []byte("ok"), nil
	})

	ctx := context.Background()

	// 1. Dry Run test
	dryRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	// Item 0 is protected and filtered out
	if len(dryRes.CleanedItems) != 3 {
		t.Errorf("expected 3 items cleaned in dry run (excluding snapshot 0), got %d", len(dryRes.CleanedItems))
	}
	if len(dryRes.Errors) != 1 {
		t.Errorf("expected 1 error for attempted deletion of protected snapshot 0, got %d", len(dryRes.Errors))
	}
	if len(executedCmds) != 0 {
		t.Errorf("DryRun should not execute any commands")
	}

	// 2. Real Clean with SnapshotSafeguard = true
	executedCmds = nil
	cleanRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false, SnapshotSafeguard: true})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
	if len(cleanRes.CleanedItems) != 3 {
		t.Errorf("expected 3 items cleaned, got %d", len(cleanRes.CleanedItems))
	}

	// Verify safeguard snapshot was created first
	if len(executedCmds) < 3 {
		t.Fatalf("expected at least 3 commands (safeguard snapshot + delete root + delete home), got %d: %v", len(executedCmds), executedCmds)
	}

	firstCmd := executedCmds[0]
	if firstCmd[0] != "snapper" || firstCmd[1] != "-c" || firstCmd[2] != "root" || firstCmd[3] != "create" {
		t.Errorf("expected first command to create safeguard snapshot, got: %v", firstCmd)
	}

	// Verify root delete command
	var foundRootDelete, foundHomeDelete bool
	for _, cmd := range executedCmds[1:] {
		if len(cmd) >= 5 && cmd[0] == "snapper" && cmd[1] == "-c" && cmd[2] == "root" && cmd[3] == "delete" {
			foundRootDelete = true
			if !strings.Contains(strings.Join(cmd, " "), "10") || !strings.Contains(strings.Join(cmd, " "), "11") {
				t.Errorf("expected root delete to contain 10 and 11, got: %v", cmd)
			}
			for _, arg := range cmd[4:] {
				if arg == "0" {
					t.Errorf("root delete must NEVER include snapshot 0, got: %v", cmd)
				}
			}
		}
		if len(cmd) >= 5 && cmd[0] == "snapper" && cmd[1] == "-c" && cmd[2] == "home" && cmd[3] == "delete" {
			foundHomeDelete = true
			if !strings.Contains(strings.Join(cmd, " "), "5") {
				t.Errorf("expected home delete to contain 5, got: %v", cmd)
			}
		}
	}

	if !foundRootDelete {
		t.Errorf("expected root snapshots delete command, not found")
	}
	if !foundHomeDelete {
		t.Errorf("expected home snapshots delete command, not found")
	}
}

func TestSnapperModule_PermissionDiagnostics(t *testing.T) {
	m := New()
	m.SetAvailable(true)
	m.SetConfigs([]string{"root"})

	// Simulate unprivileged execution where snapper returns "No permissions."
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdStr := strings.Join(args, " ")
		if strings.Contains(cmdStr, "list-configs") {
			return []byte("root\n"), nil
		}
		return []byte("No permissions.\n"), fmt.Errorf("exit status 1")
	})

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan should complete gracefully without fatal error, got: %v", err)
	}

	if len(res.Items) != 0 {
		t.Errorf("expected 0 items on permission denied, got %d", len(res.Items))
	}

	permMsg, ok := res.Metadata["permission_denied"]
	if !ok {
		t.Fatalf("expected permission_denied metadata key to be set")
	}
	if !strings.Contains(permMsg, "root privileges") || !strings.Contains(permMsg, "sudo") {
		t.Errorf("unexpected permission_denied message: %q", permMsg)
	}
}

func TestSnapperModule_PrivilegedRootScanSucceeds(t *testing.T) {
	m := New()
	m.SetAvailable(true)
	m.SetConfigs([]string{"root"})

	now := time.Now().Add(-5 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	csvOutput := fmt.Sprintf(`"number","default","active","date","type","description","used-space"
"0","yes","yes","","single","current subvolume","0 B"
"10","no","no","%s","single","root snapshot","500 MiB"
`, now)

	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdStr := strings.Join(args, " ")
		if strings.Contains(cmdStr, "list-configs") {
			return []byte("root\n"), nil
		}
		if strings.Contains(cmdStr, "list") {
			return []byte(csvOutput), nil
		}
		return nil, fmt.Errorf("unknown command: %v", args)
	})

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if _, hasPerm := res.Metadata["permission_denied"]; hasPerm {
		t.Errorf("expected permission_denied NOT to be set when scan succeeds")
	}

	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(res.Items))
	}
}
