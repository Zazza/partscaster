//go:build !windows

package workerexe

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// BindToJob: вне Windows дочерний процесс и так получает Ctrl+C из того же терминала.
func BindToJob(*os.Process) error { return nil }

func FreeBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

func lockBusy(err error) bool { return errors.Is(err, unix.EWOULDBLOCK) }
