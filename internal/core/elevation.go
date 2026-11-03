package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsPrivileged returns true if the current process is running as root (UID 0).
func IsPrivileged() bool {
	return os.Geteuid() == 0
}

// FindElevationTool locates available privilege elevation utilities (pkexec or sudo).
func FindElevationTool() string {
	if path, err := exec.LookPath("pkexec"); err == nil {
		return path
	}
	if path, err := exec.LookPath("sudo"); err == nil {
		return path
	}
	return ""
}

// RunElevated executes a command with root privileges. If already root, it runs directly.
// Otherwise, it invokes pkexec (GUI polkit agent / terminal) or sudo.
func RunElevated(ctx context.Context, name string, args ...string) ([]byte, error) {
	if IsPrivileged() {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}

	tool := FindElevationTool()
	if tool == "" {
		return nil, fmt.Errorf("operation requires root privileges, but neither pkexec nor sudo is available")
	}

	base := strings.ToLower(tool)
	var cmd *exec.Cmd

	if strings.HasSuffix(base, "pkexec") {
		pkArgs := append([]string{name}, args...)
		cmd = exec.CommandContext(ctx, tool, pkArgs...)
	} else {
		// sudo
		sudoArgs := append([]string{name}, args...)
		cmd = exec.CommandContext(ctx, tool, sudoArgs...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return out, fmt.Errorf("%s %s failed: %w (output: %s)", name, strings.Join(args, " "), err, trimmed)
		}
		return out, fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}

	return out, nil
}

// RemoveElevated removes files or directories, elevating privileges via pkexec/sudo if needed.
func RemoveElevated(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}

	// Filter out non-existent paths
	var existing []string
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return nil
	}

	// If running as root, remove directly
	if IsPrivileged() {
		for _, p := range existing {
			if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}

	// Attempt unprivileged removal first for user-owned files
	var failed []string
	for _, p := range existing {
		if err := os.RemoveAll(p); err != nil {
			failed = append(failed, p)
		}
	}

	if len(failed) == 0 {
		return nil
	}

	// Elevate to remove root-protected paths in a single batch
	rmArgs := append([]string{"-rf", "--"}, failed...)
	_, err := RunElevated(ctx, "rm", rmArgs...)
	if err != nil {
		return fmt.Errorf("elevated removal failed: %w", err)
	}

	// Double-check verification
	for _, p := range failed {
		if _, statErr := os.Lstat(p); statErr == nil {
			return fmt.Errorf("failed to remove %s even with elevation", p)
		}
	}

	return nil
}
