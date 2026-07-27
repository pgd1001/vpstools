//go:build windows

package main

func diskSpace(_ string) (total, free uint64, ok bool) {
	return 0, 0, false
}
