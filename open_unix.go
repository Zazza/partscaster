//go:build !windows

package main

import "os/exec"

func openExternal(path string) error {
	return exec.Command("xdg-open", path).Start()
}
