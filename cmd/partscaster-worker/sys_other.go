//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Программа воркера — для Windows; здесь — только чтобы пакет собирался и проверялся на других системах.

const isWindows = false

func uiLangID() uint16 { return 0 } // язык берётся из LANG (detectLang)

func showError(text string) { fmt.Fprintln(os.Stderr, tr("Ошибка:", "Error:"), text) }
func showInfo(text string)  { fmt.Fprintln(os.Stderr, text) }
func askYesNo(text string) bool {
	fmt.Fprintln(os.Stderr, text, tr("— да", "— yes"))
	return true
}

func openFile(path string) { _ = exec.Command("xdg-open", path).Start() } // нет xdg-open — путь виден в журнале

func copyText(text string) { fmt.Fprintln(os.Stderr, text) }
