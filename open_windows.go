//go:build windows

package main

func openExternal(path string) error {
	return hiddenCmd("cmd", "/c", "start", "", path).Start()
}
