package main

import (
	"os"
	"strings"
)

// Язык окна установки, трея и сообщений: русский — если язык интерфейса Windows русский, иначе английский
// (на других ОС — по LC_ALL/LC_MESSAGES/LANG); PARTSCASTER_LANG=ru|en главнее. Определяется один раз при старте.
var lang = detectLang(os.Getenv, uiLangID(), isWindows)

// detectLang — выбор языка без побочных эффектов (для тестов): переменные окружения, LANGID интерфейса Windows.
func detectLang(getenv func(string) string, winLangID uint16, isWindows bool) string {
	switch l := strings.ToLower(strings.TrimSpace(getenv("PARTSCASTER_LANG"))); l {
	case "ru", "en":
		return l
	}
	if isWindows {
		return langFromLANGID(winLangID)
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "ru") {
				return "ru"
			}
			return "en"
		}
	}
	return "en"
}

// langFromLANGID — основной язык из LANGID Windows (младшие 10 бит): 0x19 — русский, остальное — английский.
func langFromLANGID(id uint16) string {
	if id&0x3ff == 0x19 {
		return "ru"
	}
	return "en"
}

// tr — строка текущего языка: русский вариант первым, как раньше было в коде.
func tr(ru, en string) string {
	if lang == "ru" {
		return ru
	}
	return en
}
