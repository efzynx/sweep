package core

import (
	"context"
	"time"
)

// SafetyLevel defines the risk level associated with cleaning a specific item or group.
type SafetyLevel string

const (
	// SafetySafe represents low-risk items that are safe to delete without breaking anything
	// (e.g., dangling Docker images, broken package downloads, old build caches).
	SafetySafe SafetyLevel = "SAFE"

	// SafetyCaution represents moderate-risk items that might need to be re-downloaded
	// or are useful for rollback (e.g., package manager caches, older snapshots).
	SafetyCaution SafetyLevel = "CAUTION"

	// SafetyHigh represents high-risk items requiring explicit user attention and confirmation
	// (e.g., active container volumes, recent snapshots, system configuration states).
	SafetyHigh SafetyLevel = "HIGH"
)

// CleanableItem represents an individual cleanable unit discovered by a module scanner.
type CleanableItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Path        string            `json:"path,omitempty"`
	Size        int64             `json:"size"`
	Selected    bool              `json:"selected"`
	Safety      SafetyLevel       `json:"safety"`
	ModTime     time.Time         `json:"mod_time,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ScanResult aggregates all items and statistics found during a module scan.
type ScanResult struct {
	Items     []CleanableItem   `json:"items"`
	TotalSize int64             `json:"total_size"`
	ScannedAt time.Time         `json:"scanned_at"`
	Error     error             `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// RecalculateTotal updates and returns the TotalSize based on all Items in the ScanResult.
func (sr *ScanResult) RecalculateTotal() int64 {
	var total int64
	for _, item := range sr.Items {
		total += item.Size
	}
	sr.TotalSize = total
	return total
}

// SelectedItems returns a slice of items where Selected is true.
func (sr *ScanResult) SelectedItems() []CleanableItem {
	var selected []CleanableItem
	for _, item := range sr.Items {
		if item.Selected {
			selected = append(selected, item)
		}
	}
	return selected
}

// SelectedSize returns the total byte size of all selected items.
func (sr *ScanResult) SelectedSize() int64 {
	var total int64
	for _, item := range sr.Items {
		if item.Selected {
			total += item.Size
		}
	}
	return total
}

// SetAllSelected sets the Selected state for all items in the scan result.
func (sr *ScanResult) SetAllSelected(selected bool) {
	for i := range sr.Items {
		sr.Items[i].Selected = selected
	}
}

// CleanOptions specifies configuration flags for a clean operation.
type CleanOptions struct {
	DryRun            bool `json:"dry_run"`
	SnapshotSafeguard bool `json:"snapshot_safeguard"`
}

// CleanResult encapsulates the outcome and metrics of a module's clean execution.
type CleanResult struct {
	CleanedItems []CleanableItem `json:"cleaned_items"`
	FreedBytes   int64           `json:"freed_bytes"`
	Errors       []error         `json:"errors,omitempty"`
	Duration     time.Duration   `json:"duration"`
}

// Module is the common interface implemented by all subsystem scanners and cleaners.
type Module interface {
	// ID returns a unique identifier for the module (e.g. "pacman", "docker", "snapper").
	ID() string

	// Name returns a human-readable display name (e.g. "Pacman & AUR Cache").
	Name() string

	// Category returns the module category (e.g. "package_manager", "containers", "snapshots").
	Category() string

	// Description returns a brief summary of what this module targets.
	Description() string

	// IsAvailable determines whether the module can operate on the current system
	// (e.g., checks if underlying binaries exist or if runtime distro matches).
	IsAvailable() bool

	// RequiresElevation indicates whether cleaning actions require elevated root/sudo privileges.
	RequiresElevation() bool

	// Scan performs a read-only inspection of cleanable items.
	Scan(ctx context.Context) (ScanResult, error)

	// Clean executes cleanup for the provided items respecting the provided options.
	Clean(ctx context.Context, items []CleanableItem, opts CleanOptions) (CleanResult, error)
}
