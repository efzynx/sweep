package docker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/efzynx/sweep/internal/core"
)

func TestDockerModule_Metadata(t *testing.T) {
	m := New()

	if m.ID() != "docker" {
		t.Errorf("expected ID 'docker', got %q", m.ID())
	}
	if m.Name() != "Docker Engine" {
		t.Errorf("expected Name 'Docker Engine', got %q", m.Name())
	}
	if m.Category() != "containers" {
		t.Errorf("expected Category 'containers', got %q", m.Category())
	}
	if m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be false")
	}
	if m.Description() == "" {
		t.Errorf("expected non-empty description")
	}
}

func TestDockerModule_IsAvailable(t *testing.T) {
	m := New()

	m.SetDaemonChecker(func(ctx context.Context) bool {
		return false
	})
	if m.IsAvailable() {
		t.Errorf("expected IsAvailable to be false when daemon is unreachable")
	}

	m.SetDaemonChecker(func(ctx context.Context) bool {
		return true
	})
	// In an environment where docker binary is available
	if !m.IsAvailable() {
		t.Errorf("expected IsAvailable to be true when docker binary exists and daemon is up")
	}
}

func TestDockerModule_Scan(t *testing.T) {
	m := New()
	m.SetDaemonChecker(func(ctx context.Context) bool {
		return true
	})

	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdStr := strings.Join(args, " ")
		switch {
		case strings.Contains(cmdStr, "ps -a -f status=exited"):
			return []byte("c1a2b3|web-dev|25.5MB (virtual 150MB)\nc4d5e6|redis-test|10.2MB\n"), nil

		case strings.Contains(cmdStr, "images -f dangling=true"):
			return []byte("img9988|<none>:<none>|120MB\n"), nil

		case strings.Contains(cmdStr, "volume ls -q -f dangling=true"):
			return []byte("vol_temp_data\nvol_cache\n"), nil

		case strings.Contains(cmdStr, "system df -v"):
			return []byte(`
Local Volumes space usage:

VOLUME NAME   LINKS   SIZE
vol_temp_data 0       50MB
vol_cache     0       15MB
active_vol    1       100MB
`), nil

		case strings.Contains(cmdStr, "system df --format"):
			return []byte("Build Cache|300MB|250MB (83%)\n"), nil

		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	m.SetCommandRunner(mockRunner)

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(res.Items) != 6 {
		t.Fatalf("expected 6 items, got %d", len(res.Items))
	}

	itemMap := make(map[string]core.CleanableItem)
	for _, it := range res.Items {
		itemMap[it.ID] = it
	}

	// Verify containers
	c1, ok := itemMap["docker-container:c1a2b3"]
	if !ok {
		t.Errorf("container c1a2b3 not found")
	} else {
		if c1.Size == 0 {
			t.Errorf("expected parsed size for container c1")
		}
		if c1.Safety != core.SafetySafe {
			t.Errorf("expected SafetySafe for stopped container")
		}
	}

	// Verify images
	img, ok := itemMap["docker-image:img9988"]
	if !ok {
		t.Errorf("image img9988 not found")
	} else {
		if img.Size == 0 {
			t.Errorf("expected parsed size for dangling image")
		}
	}

	// Verify volumes
	v1, ok := itemMap["docker-volume:vol_temp_data"]
	if !ok {
		t.Errorf("volume vol_temp_data not found")
	} else {
		if v1.Size == 0 {
			t.Errorf("expected parsed size for volume")
		}
		if v1.Selected {
			t.Errorf("expected volume to be unselected by default")
		}
		if v1.Safety != core.SafetyCaution {
			t.Errorf("expected SafetyCaution for volume")
		}
	}

	// Verify build cache
	bc, ok := itemMap["docker-build-cache"]
	if !ok {
		t.Errorf("docker-build-cache not found")
	} else {
		if bc.Size == 0 {
			t.Errorf("expected parsed size for build cache")
		}
	}
}

func TestDockerModule_Clean(t *testing.T) {
	m := New()

	items := []core.CleanableItem{
		{
			ID:       "docker-container:c1",
			Size:     100,
			Selected: true,
			Metadata: map[string]string{
				"type":         "container",
				"container_id": "c1",
			},
		},
		{
			ID:       "docker-image:i1",
			Size:     200,
			Selected: true,
			Metadata: map[string]string{
				"type":     "image",
				"image_id": "i1",
			},
		},
		{
			ID:       "docker-volume:v1",
			Size:     300,
			Selected: true,
			Metadata: map[string]string{
				"type":        "volume",
				"volume_name": "v1",
			},
		},
		{
			ID:       "docker-build-cache",
			Size:     400,
			Selected: true,
			Metadata: map[string]string{
				"type": "build_cache",
			},
		},
	}

	var executedCommands [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdLine := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmdLine)
		return []byte("ok"), nil
	})

	ctx := context.Background()

	// 1. Dry Run test
	dryRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if len(dryRes.CleanedItems) != 4 {
		t.Errorf("expected 4 cleaned items, got %d", len(dryRes.CleanedItems))
	}
	if dryRes.FreedBytes != 1000 {
		t.Errorf("expected 1000 freed bytes, got %d", dryRes.FreedBytes)
	}
	if len(executedCommands) != 0 {
		t.Errorf("DryRun should not execute any commands, executed: %v", executedCommands)
	}

	// 2. Real clean test
	cleanRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
	if len(cleanRes.CleanedItems) != 4 {
		t.Errorf("expected 4 cleaned items, got %d", len(cleanRes.CleanedItems))
	}
	if len(cleanRes.Errors) != 0 {
		t.Errorf("expected no clean errors, got: %v", cleanRes.Errors)
	}

	if len(executedCommands) != 4 {
		t.Fatalf("expected 4 executed commands, got %d", len(executedCommands))
	}

	expectedPrefixes := [][]string{
		{"docker", "rm", "c1"},
		{"docker", "rmi", "i1"},
		{"docker", "volume", "rm", "v1"},
		{"docker", "builder", "prune", "-f"},
	}

	for i, expected := range expectedPrefixes {
		actual := executedCommands[i]
		if len(actual) != len(expected) {
			t.Errorf("cmd %d: expected length %d, got %d (%v)", i, len(expected), len(actual), actual)
			continue
		}
		for j := range expected {
			if actual[j] != expected[j] {
				t.Errorf("cmd %d arg %d: expected %s, got %s", i, j, expected[j], actual[j])
			}
		}
	}
}
