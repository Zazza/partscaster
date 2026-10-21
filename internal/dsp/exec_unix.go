//go:build !windows

package dsp

import (
	"os/exec"

	"partscaster/internal/ffmpegbin"
)

// ffmpegCmd — на unix окна консоли не мелькают; свой ffmpeg (если приложение скачало) — прежде PATH.
func ffmpegCmd(args []string) *exec.Cmd {
	return exec.Command(ffmpegbin.Path(), args...)
}
