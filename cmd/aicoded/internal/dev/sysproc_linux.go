package dev

import "syscall"

// appSysProcAttr has the kernel send the app SIGTERM when aicoded dev dies.
func appSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
