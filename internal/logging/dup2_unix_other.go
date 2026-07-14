//go:build unix && !linux

package logging

import "syscall"

func dup2(oldfd, newfd int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_DUP2, uintptr(oldfd), uintptr(newfd), 0)
	if errno != 0 {
		return errno
	}
	return nil
}
