package dev

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"aicoded.dev/framework/internal/errs"
)

// StatePath returns the private folder of the workspace rooted at root, a folder named by a hash
// of root under ~/.local/state/aicoded, without creating it.
func StatePath(root string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(home, ".local", "state", "aicoded", hex.EncodeToString(sum[:8])), nil
}

// StateDir returns the folder of StatePath. It creates the folder with mode 0700 and refuses one
// that other users can read.
func StateDir(root string) (string, error) {
	dir, err := StatePath(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return "", errs.New("E-DEV-009", fmt.Sprintf("the state folder %s can be read by other users (mode %04o)", dir, perm),
			"run: chmod 700 "+dir)
	}
	return dir, nil
}
