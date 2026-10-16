//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

// playerCommand — на macOS играет afplay (есть в системе; flac — с macOS 10.13). Громкость — множитель
// -v (1 = как есть), как --volume у pw-play. Пауза — SIGSTOP/SIGCONT, перемотка — ffmpeg-нарезкой (player.go).
func playerCommand(file string, volume float64) *exec.Cmd {
	return hiddenCmd("afplay", "-v", fmt.Sprintf("%.2f", volume), file)
}

// hiddenCmd — окон консоли на macOS нет, обычная команда.
func hiddenCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// setVolumeLive — у afplay нет управления громкостью на ходу: новая громкость действует со следующего
// запуска (перемотка, следующий трек).
func setVolumeLive(string, float64) {}

func openExternal(path string) error {
	return exec.Command("open", path).Start()
}
