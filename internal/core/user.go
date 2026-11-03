package core

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// Internal hooks for testing
var (
	getuid     = os.Getuid
	lookupUser = user.Lookup
)

// GetUserHomeDir returns the home directory of the current user.
// If running with root privileges (uid == 0) under sudo, it resolves the original
// interactive user specified in SUDO_USER (preserving user caches and configuration),
// while allowing root execution for system-level operations.
func GetUserHomeDir() (string, error) {
	if getuid() == 0 {
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUser != "" && sudoUser != "root" {
			// 1. Try lookup via os/user
			if u, err := lookupUser(sudoUser); err == nil && u != nil && u.HomeDir != "" {
				return u.HomeDir, nil
			}

			// 2. Fallback to /home/<sudoUser>
			fallback := filepath.Join("/home", sudoUser)
			if info, err := os.Stat(fallback); err == nil && info.IsDir() {
				return fallback, nil
			}
			return fallback, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if envHome := os.Getenv("HOME"); envHome != "" {
			return envHome, nil
		}
		if err != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", err)
		}
		return "", fmt.Errorf("user home directory is empty")
	}

	return home, nil
}
