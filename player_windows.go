//go:build windows

package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// playerCommand — воспроизведение через WPF MediaPlayer (PowerShell, STA).
// Media Foundation играет flac/mp3/wav; kill процесса (killLocked) — это стоп.
// Пауза-возобновление с позиции не поддерживается: resume стартует заново.
// Живая громкость: рядом с файлом лежит <file>.vol (0..1), цикл плейера
// перечитывает его и применяет к $m.Volume.
// Цикл обязан ЗАВЕРШАТЬСЯ по концу носителя (позиция у длительности, подстраховка
// по стенным часам) или при застрявшей позиции (файл не открылся): иначе процесс
// живёт вечно, cmd.Wait не возвращается и таймер уезжает за конец трека.
func playerCommand(file string, volume float64) *exec.Cmd {
	uri := (&url.URL{Scheme: "file", Path: "/" + strings.ReplaceAll(file, "\\", "/")}).String()
	writeVolumeFile(file, volume)
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName PresentationCore; `+
			`$m = New-Object System.Windows.Media.MediaPlayer; `+
			`$m.Volume = `+strconv.FormatFloat(volume, 'f', 2, 64)+`; `+
			`$m.Open([Uri]%q); `+
			`$w = [Diagnostics.Stopwatch]::StartNew(); `+
			`while ($m.NaturalDuration.TimeSpan.TotalSeconds -le 0 -and $w.Elapsed.TotalSeconds -lt 5) { Start-Sleep -Milliseconds 50 }; `+
			`$dur = $m.NaturalDuration.TimeSpan.TotalSeconds; `+
			`$w.Restart(); `+
			`$m.Play(); `+
			`$vf = '%s'; `+
			`$last = -1.0; $stall = 0; `+
			`while ($true) { `+
			`if (Test-Path $vf) { $nv = [double](Get-Content $vf -ErrorAction SilentlyContinue); `+
			`if ($null -ne $nv -and $nv -ge 0 -and $nv -le 1 -and $nv -ne $m.Volume) { $m.Volume = $nv } }; `+
			`$pos = $m.Position.TotalSeconds; `+
			`if ($dur -gt 0 -and $pos -ge ($dur - 0.05)) { break }; `+
			`if ($dur -gt 0 -and $w.Elapsed.TotalSeconds -ge ($dur + 2)) { break }; `+
			`if ([math]::Abs($pos - $last) -lt 0.001) { $stall = $stall + 1 } else { $stall = 0; $last = $pos }; `+
			`if ($stall -ge 8) { break }; `+
			`Start-Sleep -Milliseconds 400 }`, uri, volumeFile(file))
	// powershell.exe (Windows PowerShell 5.1) — MediaPlayer требует STA-поток.
	// Абсолютный путь: у GUI-процесса PATH и SystemRoot могут быть пусты.
	candidates := []string{
		filepath.Join(os.Getenv("SystemRoot"), `System32\WindowsPowerShell\v1.0\powershell.exe`),
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
	}
	psExe := "powershell.exe"
	for _, c := range candidates {
		if c != "" && c != `\System32\WindowsPowerShell\v1.0\powershell.exe` {
			if _, err := os.Stat(c); err == nil {
				psExe = c
				break
			}
		}
	}
	cmd := hiddenCmd(psExe, "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-Command", ps)
	return cmd
}

// hiddenCmd — команда без окна консоли: у GUI-процесса любой консольный
// потомок (powershell/ffmpeg/cmd) иначе мелькает терминалом.
func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func volumeFile(file string) string { return file + ".vol" }

func writeVolumeFile(file string, v float64) {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	_ = os.WriteFile(volumeFile(file), []byte(fmt.Sprintf("%.2f", v)), 0o644)
}
