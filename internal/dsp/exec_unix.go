//go:build !windows

package dsp

import (
	"os/exec"

	"partscaster/internal/ffmpegbin"
)

// ffmpegCmd — на unix окна консоли не мелькают; какой ffmpeg — решает ffmpegbin.Path(): переменная → PATH →
// Homebrew (macOS) → своя папка.
func ffmpegCmd(args []string) *exec.Cmd {
	return exec.Command(ffmpegbin.Path(), args...)
}
