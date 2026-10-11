//go:build windows

package dsp

import "time"

// childCPU — на Windows недоступно: замер скорости идёт по настенному времени.
func childCPU() (time.Duration, bool) { return 0, false }
