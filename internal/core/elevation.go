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

// ElevatedAction represents a discrete privileged command to be executed in a single root batch.
type ElevatedAction struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
}

// ExecuteBatch runs multiple privileged actions under a SINGLE pkexec/sudo elevation invocation.
// This guarantees that the user is prompted for their password at most once for all privileged actions combined.
func ExecuteBatch(ctx context.Context, actions []ElevatedAction) (map[string]error, error) {
	results := make(map[string]error)
	if len(actions) == 0 {
		return results, nil
	}

	// If already running with root privileges, execute directly without elevation helper
	if IsPrivileged() {
		for _, act := range actions {
			cmd := exec.CommandContext(ctx, act.Command, act.Args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				results[act.ID] = fmt.Errorf("%s failed: %w (output: %s)", act.Command, err, strings.TrimSpace(string(out)))
			} else {
				results[act.ID] = nil
			}
		}
		return results, nil
	}

	tool := FindElevationTool()
	if tool == "" {
		return results, fmt.Errorf("root privileges required, but neither pkexec nor sudo is available")
	}

	// Create a single temporary script file containing all privileged actions
	tmpFile, err := os.CreateTemp("", "sweep-elevated-*.sh")
	if err != nil {
		return results, fmt.Errorf("failed to create temporary elevation script: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("# Sweep Automated Privileged Execution Script\n")

	for _, act := range actions {
		sb.WriteString(fmt.Sprintf("echo '===SWEEP_START:%s==='\n", act.ID))
		var escapedArgs []string
		for _, arg := range act.Args {
			escapedArgs = append(escapedArgs, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", act.Command, strings.Join(escapedArgs, " ")))
		sb.WriteString(fmt.Sprintf("echo '===SWEEP_EXIT:'$?':%s==='\n", act.ID))
	}

	if err := tmpFile.Chmod(0700); err != nil {
		_ = tmpFile.Close()
		return results, err
	}
	if _, err := tmpFile.WriteString(sb.String()); err != nil {
		_ = tmpFile.Close()
		return results, err
	}
	_ = tmpFile.Close()

	// Execute via pkexec or sudo ONCE
	base := strings.ToLower(tool)
	var cmd *exec.Cmd
	if strings.HasSuffix(base, "pkexec") {
		cmd = exec.CommandContext(ctx, tool, "/bin/sh", tmpPath)
	} else {
		cmd = exec.CommandContext(ctx, tool, "/bin/sh", tmpPath)
	}

	combinedOut, execErr := cmd.CombinedOutput()
	outputStr := string(combinedOut)

	// If the entire tool failed (e.g., user dismissed polkit prompt)
	if execErr != nil && !strings.Contains(outputStr, "===SWEEP_START:") {
		trimmed := strings.TrimSpace(outputStr)
		if trimmed != "" {
			return results, fmt.Errorf("privilege elevation failed: %w (output: %s)", execErr, trimmed)
		}
		return results, fmt.Errorf("privilege elevation was canceled or failed: %w", execErr)
	}

	// Parse individual action results from output
	lines := strings.Split(outputStr, "\n")
	currentID := ""
	var currentOutput []string

	for _, line := range lines {
		if strings.HasPrefix(line, "===SWEEP_START:") && strings.HasSuffix(line, "===") {
			currentID = strings.TrimSuffix(strings.TrimPrefix(line, "===SWEEP_START:"), "===")
			currentOutput = nil
			continue
		}
		if strings.HasPrefix(line, "===SWEEP_EXIT:") && strings.HasSuffix(line, "===") {
			parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "===SWEEP_EXIT:"), "==="), ":")
			if len(parts) == 2 {
				exitCode := parts[0]
				actionID := parts[1]
				if exitCode == "0" {
					results[actionID] = nil
				} else {
					outMsg := strings.TrimSpace(strings.Join(currentOutput, " "))
					if outMsg != "" {
						results[actionID] = fmt.Errorf("exit code %s: %s", exitCode, outMsg)
					} else {
						results[actionID] = fmt.Errorf("exit code %s", exitCode)
					}
				}
			}
			currentID = ""
			continue
		}
		if currentID != "" {
			currentOutput = append(currentOutput, line)
		}
	}

	// For any action not reached (if script terminated early)
	for _, act := range actions {
		if _, ok := results[act.ID]; !ok {
			if execErr != nil {
				results[act.ID] = execErr
			} else {
				results[act.ID] = fmt.Errorf("action not executed")
			}
		}
	}

	return results, nil
}

// RunElevated executes a single command with root privileges. If already root, it runs directly.
// Otherwise, it invokes pkexec or sudo.
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
	res, err := ExecuteBatch(ctx, []ElevatedAction{{
		ID:      "remove_paths",
		Command: "rm",
		Args:    rmArgs,
	}})
	if err != nil {
		return fmt.Errorf("elevated removal failed: %w", err)
	}
	if actErr := res["remove_paths"]; actErr != nil {
		return fmt.Errorf("elevated removal failed: %w", actErr)
	}

	// Double-check verification
	for _, p := range failed {
		if _, statErr := os.Lstat(p); statErr == nil {
			return fmt.Errorf("failed to remove %s even with elevation", p)
		}
	}

	return nil
}
