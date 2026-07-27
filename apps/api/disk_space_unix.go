//go:build !windows

package main

import (
	"syscall"
)

func diskSpace(path string) (total, free uint64, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	return uint64(stat.Blocks) * uint64(stat.Bsize), uint64(stat.Bavail) * uint64(stat.Bsize), true
}
