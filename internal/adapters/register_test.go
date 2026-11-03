package adapters

import (
	"testing"

	"github.com/efzynx/sweep/internal/core"
)

func TestRegisterDefaultAdapters(t *testing.T) {
	reg := core.NewRegistry()

	RegisterDefaultAdapters(reg)

	expectedIDs := []string{"pacman", "docker", "flatpak", "snapper", "apt", "dnf", "custom"}
	for _, id := range expectedIDs {
		if _, ok := reg.Get(id); !ok {
			t.Errorf("expected module %s to be registered", id)
		}
	}

	// Calling a second time should not error or duplicate
	RegisterDefaultAdapters(reg)

	modules := reg.Modules()
	if len(modules) != len(expectedIDs) {
		t.Errorf("expected %d modules, got %d", len(expectedIDs), len(modules))
	}
}
