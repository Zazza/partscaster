package main

import (
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"

	"partscaster/internal/workerexe"
)

const (
	isWindows = true
	idYes     = 6 // MessageBox: нажата «Да»
)

// uiLangID — язык интерфейса Windows (LANGID) для выбора языка окна и трея.
func uiLangID() uint16 {
	r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	return uint16(r)
}

func caption() string { return tr("Partscaster — воркер", "Partscaster — worker") }

func messageBox(text string, flags uint32) int32 {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(caption())
	ret, _ := windows.MessageBox(0, t, c, flags) // окно-сообщение: ошибка показа — показать нечем
	return ret
}

func showError(text string) { messageBox(text, windows.MB_OK|windows.MB_ICONERROR) }
func showInfo(text string)  { messageBox(text, windows.MB_OK|windows.MB_ICONINFORMATION) }

func askYesNo(text string) bool {
	return messageBox(text, windows.MB_YESNO|windows.MB_ICONQUESTION|windows.MB_DEFBUTTON2) == idYes
}

// openFile — файл в Блокноте (журнал — обычный текст).
func openFile(path string) {
	cmd := exec.Command("notepad.exe", path)
	_ = cmd.Start() // не открылся — путь журнала есть в описании папки
}

// copyText — в буфер обмена через clip.exe (адрес — латиница и цифры).
func copyText(text string) {
	cmd := exec.Command("clip")
	cmd.Stdin = strings.NewReader(text)
	workerexe.HideConsole(cmd)
	_ = cmd.Run() // не скопировалось — адрес виден в меню
}
