//go:build !linux

package dev

import "syscall"

// appSysProcAttr has no attributes on systems without a parent-death signal.
func appSysProcAttr() *syscall.SysProcAttr { return nil }
