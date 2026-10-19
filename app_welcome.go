package main

import (
	"time"

	"partscaster/internal/welcome"
)

// YuePlayWelcomeClip — клип «было/стало» окна знакомства во встроенном плеере, с секунды startSec
// (переключение версии во время игры продолжает с того же места). durSec — длина клипа для полосы позиции.
func (a *App) YuePlayWelcomeClip(name string, startSec, durSec float64) error {
	data, err := welcome.Clip(name)
	if err != nil {
		return err
	}
	if err := a.player.Load(welcomeClipID, data, time.Duration(durSec*float64(time.Second))); err != nil {
		return err
	}
	if startSec > 0 && startSec < durSec {
		// перемотка сама запускает плеер с позиции — без лишнего старта с нуля
		return a.player.Seek(time.Duration(startSec * float64(time.Second)))
	}
	return a.player.Play()
}
