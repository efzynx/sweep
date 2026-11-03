package journal

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

func TestModuleMetadata(t *testing.T) {
	m := New()

	if m.ID() != "journal" {
		t.Errorf("expected ID journal, got %s", m.ID())
	}
	if m.Name() != "Systemd Journal Logs" {
		t.Errorf("expected Name 'Systemd Journal Logs', got %s", m.Name())
	}
	if m.Category() != "system_logs" {
		t.Errorf("expected Category system_logs, got %s", m.Category())
	}
	if !m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be true for systemd journal")
	}
}

func toBytes(val float64, mult int64) int64 {
	return int64(val * float64(mult))
}

func TestParseDiskUsage(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantBytes int64
		wantErr   bool
	}{
		{
			name:      "Archived and active journals (MB)",
			output:    "Archived and active journals take up 44.9M in the file system.",
			wantBytes: toBytes(44.9, 1024*1024),
			wantErr:   false,
		},
		{
			name:      "Journals on disk (GB)",
			output:    "Archived and active journals take up 1.2G on disk.",
			wantBytes: toBytes(1.2, 1024*1024*1024),
			wantErr:   false,
		},
		{
			name:      "Journals take up (KB)",
			output:    "Journals take up 512.0K in the file system.",
			wantBytes: 512 * 1024,
			wantErr:   false,
		},
		{
			name:      "Journals in bytes",
			output:    "Archived and active journals take up 1024B in the file system.",
			wantBytes: 1024,
			wantErr:   false,
		},
		{
			name:      "No journals found",
			output:    "No journals found.",
			wantBytes: 0,
			wantErr:   false,
		},
		{
			name:      "Empty output",
			output:    "",
			wantBytes: 0,
			wantErr:   true,
		},
		{
			name:      "Unparseable garbage",
			output:    "systemd-journal error: corrupted",
			wantBytes: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDiskUsage(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDiskUsage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantBytes {
				t.Errorf("ParseDiskUsage() got = %d, want %d", got, tt.wantBytes)
			}
		})
	}
}

func TestParseVacuumFreed(t *testing.T) {
	output := `Vacuuming done, freed 0B of archived journals from /run/log/journal.
Vacuuming done, freed 12.5M of archived journals from /var/log/journal.
Vacuuming done, freed 2.0M of archived journals from /var/log/journal/c17a8ce2c0af459a8cf1cc4e174961d5.`

	got, err := ParseVacuumFreed(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := toBytes(12.5, 1024*1024) + toBytes(2.0, 1024*1024)
	if got != expected {
		t.Errorf("expected total freed %d, got %d", expected, got)
	}
}

func TestScan_NotAvailable(t *testing.T) {
	m := New()
	m.SetSystemdRunning(func() bool { return false })

	if m.IsAvailable() {
		t.Errorf("expected IsAvailable to be false when systemd is not running")
	}

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("expected 0 items when unavailable, got %d", len(res.Items))
	}
}

func TestScan_SizeExcess(t *testing.T) {
	m := New()
	m.SetSystemdRunning(func() bool { return true })
	m.SetJournalDirs([]string{t.TempDir()}) // empty dir

	// Mock journalctl --disk-usage returning 80MB (exceeds default 50MB limit)
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if args[0] == "--disk-usage" {
			return []byte("Archived and active journals take up 80.0M in the file system."), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	})

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}

	item := res.Items[0]
	if item.ID != "journal:vacuum" {
		t.Errorf("expected ID journal:vacuum, got %s", item.ID)
	}

	// 80MB - 50MB = 30MB
	expectedVacuumable := int64((80 - 50) * 1024 * 1024)
	if item.Size != expectedVacuumable {
		t.Errorf("expected vacuumable size %d, got %d", expectedVacuumable, item.Size)
	}
	if item.Safety != core.SafetySafe {
		t.Errorf("expected safety SAFE, got %s", item.Safety)
	}
}

func TestScan_ArchivedFilesOlderThanRetention(t *testing.T) {
	m := New()
	m.SetSystemdRunning(func() bool { return true })

	tempJournalDir := t.TempDir()
	machineDir := filepath.Join(tempJournalDir, "machinedir123")
	if err := os.MkdirAll(machineDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Active journal (should NOT be vacuumed)
	activeJournal := filepath.Join(machineDir, "system.journal")
	if err := os.WriteFile(activeJournal, make([]byte, 5*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	// Recent archived journal (3 days old, within 7 day retention)
	recentArchived := filepath.Join(machineDir, "system@recent-123.journal")
	if err := os.WriteFile(recentArchived, make([]byte, 5*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}
	recentTime := time.Now().AddDate(0, 0, -3)
	_ = os.Chtimes(recentArchived, recentTime, recentTime)

	// Old archived journal (14 days old, exceeds 7 day retention)
	oldArchived := filepath.Join(machineDir, "system@old-456.journal")
	oldSize := int64(8 * 1024 * 1024) // 8MB
	if err := os.WriteFile(oldArchived, make([]byte, oldSize), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -14)
	_ = os.Chtimes(oldArchived, oldTime, oldTime)

	m.SetJournalDirs([]string{tempJournalDir})
	m.SetRetentionDays(7)
	m.SetVacuumSizeLimit(100*1024*1024, "100M") // High limit so only age triggers

	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Archived and active journals take up 18.0M in the file system."), nil
	})

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}

	if res.Items[0].Size != oldSize {
		t.Errorf("expected vacuumable size to match old archived journal size %d, got %d", oldSize, res.Items[0].Size)
	}
}

func TestClean_DryRun(t *testing.T) {
	m := New()
	m.SetSystemdRunning(func() bool { return true })

	executed := false
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		executed = true
		return []byte("Vacuuming done, freed 10.0M"), nil
	})

	item := core.CleanableItem{
		ID:   "journal:vacuum",
		Name: "Systemd Journal Logs",
		Size: 10 * 1024 * 1024,
	}

	cleanRes, err := m.Clean(context.Background(), []core.CleanableItem{item}, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if executed {
		t.Errorf("journalctl was executed during dry run!")
	}
	if len(cleanRes.CleanedItems) != 1 {
		t.Errorf("expected 1 cleaned item in dry run, got %d", len(cleanRes.CleanedItems))
	}
	if cleanRes.FreedBytes != item.Size {
		t.Errorf("expected freed bytes %d, got %d", item.Size, cleanRes.FreedBytes)
	}
}

func TestClean_RealExecution(t *testing.T) {
	m := New()
	m.SetSystemdRunning(func() bool { return true })
	m.SetRetentionDays(7)
	m.SetVacuumSizeLimit(50*1024*1024, "50M")

	var capturedArgs []string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		capturedArgs = args
		return []byte("Vacuuming done, freed 15.0M of archived journals from /var/log/journal."), nil
	})

	item := core.CleanableItem{
		ID:   "journal:vacuum",
		Name: "Systemd Journal Logs",
		Size: 15 * 1024 * 1024,
	}

	cleanRes, err := m.Clean(context.Background(), []core.CleanableItem{item}, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if len(capturedArgs) < 2 {
		t.Fatalf("expected args passed to journalctl, got %v", capturedArgs)
	}

	foundTimeArg := false
	foundSizeArg := false
	for _, arg := range capturedArgs {
		if strings.HasPrefix(arg, "--vacuum-time=7d") {
			foundTimeArg = true
		}
		if strings.HasPrefix(arg, "--vacuum-size=50M") {
			foundSizeArg = true
		}
	}

	if !foundTimeArg {
		t.Errorf("expected --vacuum-time=7d in arguments, got %v", capturedArgs)
	}
	if !foundSizeArg {
		t.Errorf("expected --vacuum-size=50M in arguments, got %v", capturedArgs)
	}

	expectedFreed := int64(15.0 * 1024 * 1024)
	if cleanRes.FreedBytes != expectedFreed {
		t.Errorf("expected freed bytes %d, got %d", expectedFreed, cleanRes.FreedBytes)
	}
}
