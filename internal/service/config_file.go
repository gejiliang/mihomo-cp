package service

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// WriteConfigFile atomically replaces a regular config file with private (0600)
// content. Existing ownership is retained so a privileged control plane does
// not accidentally take ownership away from the mihomo service account.
// Symlinks are rejected rather than silently replacing the link itself.
func WriteConfigFile(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if info != nil && !info.Mode().IsRegular() {
		return fmt.Errorf("config path must be a regular file")
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".mihomo-config-*")
	if err != nil {
		return fmt.Errorf("create private config: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	if info != nil {
		owner := info.Sys().(*syscall.Stat_t)
		created, err := file.Stat()
		if err != nil {
			return err
		}
		current := created.Sys().(*syscall.Stat_t)
		if owner.Uid != current.Uid || owner.Gid != current.Gid {
			if err := file.Chown(int(owner.Uid), int(owner.Gid)); err != nil {
				return fmt.Errorf("preserve config ownership: %w", err)
			}
		}
	}
	// Enforce exact permissions even under a restrictive process umask.
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure config permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write private config: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync private config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close private config: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
