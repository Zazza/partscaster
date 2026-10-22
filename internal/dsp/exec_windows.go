//go:build windows

package dsp

import (
	"os/exec"
	"syscall"

	"partscaster/internal/ffmpegbin"
)

// ffmpegCmd — ffmpeg без окна консоли: у GUI-процесса оно иначе мелькает
// на каждом применении эффектов/перемотке. Какой ffmpeg — решает ffmpegbin.Path(): переменная → PATH →
// своя папка (скачанный приложением).
func ffmpegCmd(args []string) *exec.Cmd {
	cmd := exec.Command(ffmpegbin.Path(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}
