//go:build !windows

package dsp

import (
	"syscall"
	"time"
)

// childCPU — суммарное процессорное время (user+sys) завершённых дочерних процессов
// теста. Разница до и после вызова — время ffmpeg этого вызова: в отличие от
// настенного, оно не растёт, когда параллельно идут тесты других пакетов.
func childCPU() (time.Duration, bool) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &ru); err != nil {
		return 0, false
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano()), true
}
