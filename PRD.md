# Product Requirements Document (PRD) - Sweep

## 1. Executive Summary
Sweep is a modern, modular, cross-distribution Linux system maintenance and cleanup tool. Unlike legacy cleaners (such as BleachBit or unmaintained Stacer) that focus primarily on browser caches and desktop thumbnails, Sweep is designed for modern developer and power-user workflows: addressing container bloat (Docker/Podman), Copy-on-Write storage snapshots (Btrfs/ZFS), modern packaging layers (Flatpak, Snap), and distribution package managers (Pacman/AUR, APT, DNF, Zypper).

Development is executed in progressive phases:
- Phase 1: Core Engine & Interactive TUI (Terminal User Interface).
- Phase 2: Multi-distro adapters & advanced developer cache scanning.
- Phase 3: Native Desktop GUI (Tauri / Slint) built on top of the shared core engine.

---

## 2. Core Philosophy & Guiding Principles
1. Safe by Default:
   - Read-only scanning by default without requiring elevated privileges (`sudo`).
   - Clear dry-run preview displaying exact files, IDs, and estimated reclaimable space.
   - Snapshot safety hook: Optional automatic Btrfs/Snapper safeguard snapshot prior to any destructive operation.
2. Modular & Cross-Distribution:
   - Pluggable Adapter Pattern: Core cleaner logic must not hardcode distribution-specific commands.
   - Runtime distro & package manager auto-detection (/etc/os-release, command binary availability).
3. Zero Runtime Bloat:
   - Single standalone binary, low memory footprint, fast cold startup (<100ms).
4. Keyboard-Driven UX:
   - Intuitive TUI (Vim-style navigation, fuzzy filtering, spacebar toggle, bulk actions).

---

## 3. Architecture & System Modules

### 3.1 Layer Separation
```
+-------------------------------------------------------+
| Presentation Layer: TUI (Bubbletea) -> Future GUI    |
+-------------------------------------------------------+
| Service / Orchestrator Layer (Task Runner & Sudo Exec)|
+-------------------------------------------------------+
| Module Adapter Interface (Scanner + Cleaner)          |
|  - PackageManager (Pacman, APT, DNF, Zypper)          |
|  - Containers (Docker, Podman)                        |
|  - Sandboxed Apps (Flatpak, Snap)                     |
|  - Filesystems (Btrfs / Snapper, ZFS)                 |
|  - DevCaches (Node, Cargo, Pip, Composer)             |
+-------------------------------------------------------+
| OS & Process Execution Layer                          |
+-------------------------------------------------------+
```

### 3.2 Standard Module Interface
Each module adheres to a standard contract:
- `ID() string` (e.g., "pacman", "docker", "btrfs")
- `Name() string`
- `IsAvailable() bool` (checks binaries & runtime environment)
- `RequiresElevation() bool`
- `Scan(ctx) (ScanResult, error)`
- `Clean(ctx, items []CleanableItem, dryRun bool) (CleanResult, error)`

---

## 4. Feature Specifications by Category

### 4.1 Package Managers (Distro-Agnostic Engine)
- Arch / CachyOS (Phase 1 Initial):
  * Pacman package cache pruning (configurable retention, default keep 1 version via paccache).
  * Incomplete / broken download cleanup (`/var/cache/pacman/pkg/download-*`).
  * Orphan package detection & selective removal (`pacman -Qtdq`).
  * Paru / Yay AUR build directory cache pruning (`~/.cache/paru/clone`).
- Debian / Ubuntu (Phase 2):
  * APT cache clean (`apt-get clean / autoclean`).
  * Orphan packages (`apt-get autoremove --dry-run`).
- Fedora / RHEL (Phase 2):
  * DNF cache cleanup (`dnf clean all`).
  * Old kernel cleanup.

### 4.2 Containers & Virtualization
- Docker & Podman:
  * Stopped containers.
  * Dangling & unreferenced images.
  * Unused volumes.
  * BuildKit build caches.
  * Detailed item breakdown (name, image hash, virtual size, last used timestamp).

### 4.3 Copy-on-Write & Snapshots (Btrfs / Snapper)
- Snapper snapshot management:
  * List snapshots for root (`/`) and home (`/home`).
  * Categorize by type (pre/post, timeline, single).
  * Filter snapshots by age (> 3 days, > 7 days, > 30 days).
  * Safe bulk-deletion excluding active/current subvolume and boot snapshots.
- Btrfs usage & allocation report:
  * Differentiate between allocated chunks and unallocated disk capacity.

### 4.4 Sandboxed Packaging
- Flatpak:
  * Unused runtimes and orphaned dependencies (`flatpak list --unused`).
  * User-level and system-level flatpak cache cleanup.
- Snap (Phase 2):
  * Old disabled snap revisions (`snap list --all`).

### 4.5 Developer Build Caches (Phase 2)
- Node.js: Global `~/.npm/_cacache`, `~/.yarn/cache`, `~/.pnpm-store`.
- Rust / Cargo: `~/.cargo/registry/cache`, `target/` directories idle > 30 days.
- Python: `~/.cache/pip`, uv cache.
- PHP: Composer cache (`~/.cache/composer`).

---

## 5. User Interface & Workflow (Phase 1: TUI)

### 5.1 TUI Layout (Split View)
- Header: App title, active distro detection, overall disk usage meter (`/` used vs free).
- Left Sidebar (Modules List):
  * List of detected modules with indicator badge (e.g. `[x] Docker: 9.7 GB`, `[ ] Pacman: 4.5 GB`).
  * Checkbox status for batch operations.
- Right Main Area (Item Inspector):
  * Tabular or tree view of cleanable items for the active module.
  * Details: Item name, date created/modified, disk footprint, safety level (Safe / Caution / High).
- Footer / Command Bar:
  * Navigation hints: `[Tab]` switch pane | `[Space]` select | `[a]` select all | `[s]` scan | `[d]` dry-run | `[Enter]` clean | `[q]` quit.

### 5.2 Elevation Flow
- Scanning runs in user context.
- When an operation requires root privileges, the TUI issues a targeted privilege elevation prompt (`sudo` or `pkexec`) only for the batch execution step, retaining terminal focus without crashing the TUI layout.

---

## 6. Roadmap & Milestone Plan

### Milestone 1: Core & Scanner Engine (Go)
- Project scaffold, logging, configuration (`~/.config/sweep/config.yaml`).
- Base interface definitions for Module and Item.
- Detection engine for Arch/CachyOS (Pacman, Docker, Flatpak, Snapper).
- Accurate byte-size calculation & unit formatting (GB/MB).

### Milestone 2: Interactive TUI (Bubbletea + Lipgloss)
- Responsive split-view TUI component.
- Selection state manager (toggle individual items or whole categories).
- Dry-run confirmation modal.
- Progress bar and live execution logging during clean operations.

### Milestone 3: Multi-Distribution Abstraction (Phase 2)
- Modular loader supporting APT, DNF, Zypper.
- Configuration to toggle specific modules or add custom directory scanning rules.
- Automated tests across containerized Linux distros.

### Milestone 4: Native GUI (Phase 3)
- Decouple engine as a shared library / daemon / RPC service.
- Build lightweight frontend (Tauri + Svelte or Slint + Rust).
- Desktop notification and background disk health alerts.

---

## 7. Tech Stack Selection
- Language: Go (v1.23+)
  * Single statically linked binary without glibc version traps.
  * Superior concurrency model for simultaneous scanning across independent system modules.
  * Standard ecosystem for TUI: `charmbracelet/bubbletea`, `lipgloss`, `bubbles`.
- Alternative considered: Rust (Ratatui). Go is prioritized for faster development velocity and simple cross-compilation.
