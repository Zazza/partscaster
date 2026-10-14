//go:build windows

package main

import (
	"strings"
	"testing"
)

// scriptOf — текст PowerShell-скрипта плейера (последний аргумент команды).
func scriptOf(t *testing.T, file string) string {
	t.Helper()
	cmd := playerCommand(file, 0.8)
	if len(cmd.Args) < 2 {
		t.Fatalf("Args: %v", cmd.Args)
	}
	return cmd.Args[len(cmd.Args)-1]
}

// Регрессия «таймер уезжает за конец трека, кнопка не возвращается в ▶»:
// цикл плейера обязан выходить по концу носителя и при застрявшей позиции
// (файл не открылся) — иначе процесс живёт вечно и cmd.Wait не возвращается.
func TestPlayerScriptExitsOnMediaEnd(t *testing.T) {
	s := scriptOf(t, `C:\tmp\yue-1-2.flac`)
	if !strings.Contains(s, "$dur = $m.NaturalDuration.TimeSpan.TotalSeconds") {
		t.Error("в скрипте нет длительности носителя — конец не определить")
	}
	if !strings.Contains(s, "$pos -ge ($dur - 0.05)") {
		t.Error("нет выхода при позиции у длительности — трек доиграл, а процесс живёт")
	}
	if !strings.Contains(s, "$w.Elapsed.TotalSeconds -ge ($dur + 2)") {
		t.Error("нет подстраховки по стенным часам — позиция может не двигаться без Dispatcher")
	}
	if !strings.Contains(s, "$stall") || !strings.Contains(s, "if ($stall -ge 8) { break }") {
		t.Error("нет выхода при застрявшей позиции — сбой открытия оставляет процесс навсегда")
	}
	if !strings.Contains(s, "$w.Restart()") {
		t.Error("секундомер не перезапускается перед Play — подстраховка отрежет хвост на времени открытия")
	}
}

// Живая громкость остаётся: цикл перечитывает <file>.vol и применяет к плееру.
func TestPlayerScriptKeepsLiveVolume(t *testing.T) {
	s := scriptOf(t, `C:\tmp\yue-1-2.flac`)
	if !strings.Contains(s, `Test-Path $vf`) || !strings.Contains(s, `$m.Volume = $nv`) {
		t.Error("в цикле нет перечитывания файла громкости")
	}
	if !strings.Contains(s, `C:\tmp\yue-1-2.flac.vol`) {
		t.Error("путь файла громкости не рядом с носителем")
	}
}
