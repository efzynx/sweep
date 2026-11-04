package adapters

import (
	"github.com/efzynx/sweep/internal/adapters/apt"
	"github.com/efzynx/sweep/internal/adapters/custom"
	"github.com/efzynx/sweep/internal/adapters/devcache"
	"github.com/efzynx/sweep/internal/adapters/dnf"
	"github.com/efzynx/sweep/internal/adapters/docker"
	"github.com/efzynx/sweep/internal/adapters/flatpak"
	"github.com/efzynx/sweep/internal/adapters/journal"
	"github.com/efzynx/sweep/internal/adapters/pacman"
	"github.com/efzynx/sweep/internal/adapters/snap"
	"github.com/efzynx/sweep/internal/adapters/snapper"
	"github.com/efzynx/sweep/internal/adapters/usercache"
	"github.com/efzynx/sweep/internal/adapters/zypper"
	"github.com/efzynx/sweep/internal/core"
)

// RegisterDefaultAdapters registers all built-in subsystem adapters
// into the specified registry. If r is nil, core.DefaultRegistry is targeted.
func RegisterDefaultAdapters(r *core.Registry) {
	if r == nil {
		r = core.DefaultRegistry
	}

	modules := []core.Module{
		pacman.New(),
		docker.New(),
		flatpak.New(),
		snapper.New(),
		apt.New(),
		dnf.New(),
		zypper.New(),
		custom.New(),
		devcache.New(),
		snap.New(),
		usercache.New(),
		journal.New(),
	}

	for _, m := range modules {
		if _, exists := r.Get(m.ID()); !exists {
			_ = r.Register(m)
		}
	}
}
