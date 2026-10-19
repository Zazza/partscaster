//go:build !windows

package workerexe

import (
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

// BindSelfToJob: вне Windows не нужно (программа воркера — для Windows; здесь — только тесты и сборка).
func BindSelfToJob() error { return nil }

// KillTreeOnClose: вне Windows — завершить сам процесс.
func KillTreeOnClose(p *os.Process) (func(), error) { return func() { _ = p.Kill() }, nil }

// HideConsole: вне Windows отдельных консольных окон у дочерних процессов нет.
func HideConsole(*exec.Cmd) {}

func FreeBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

func lockBusy(err error) bool { return errors.Is(err, unix.EWOULDBLOCK) }
