package welcome

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ТК3: встроенный клип отдаётся байтами по имени; имя не из набора, путь с каталогом,
// выход наверх и пустое имя — ошибка.
func TestClipKnownNameReturnsBytes(t *testing.T) {
	t.Parallel()
	names := Names()
	if len(names) == 0 {
		t.Fatal("Names() пуст: встроенных клипов нет")
	}
	for _, n := range names {
		b, err := Clip(n)
		if err != nil {
			t.Errorf("Clip(%q): %v", n, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("Clip(%q): пустые байты", n)
		}
	}
}

func TestClipBadNamesAreErrors(t *testing.T) {
	t.Parallel()
	bad := []string{"", "../x", "clips/x", "no-such-clip.mp3", "/etc/passwd", "..", "."}
	if names := Names(); len(names) > 0 {
		// настоящее имя, но с каталогом или выходом наверх — тоже ошибка
		bad = append(bad, "clips/"+names[0], "../"+names[0], "./"+names[0])
	}
	for _, n := range bad {
		if b, err := Clip(n); err == nil {
			t.Errorf("Clip(%q): want ошибку, got %d байт", n, len(b))
		}
	}
}

func TestNamesWithoutDirAndUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, n := range Names() {
		if n == "" || strings.ContainsAny(n, `/\`) {
			t.Errorf("имя %q: want имя файла без каталога", n)
		}
		if seen[n] {
			t.Errorf("имя %q повторяется", n)
		}
		seen[n] = true
	}
}

// ТК4: набор встроенных файлов и список клипов в welcomeClips.json совпадают в обе стороны.
func TestNamesMatchWelcomeClipsJSON(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../frontend/src/welcomeClips.json")
	if err != nil {
		t.Fatalf("welcomeClips.json: %v", err)
	}
	var data struct {
		Steps []struct {
			ID    string `json:"id"`
			Pairs []struct {
				ID     string `json:"id"`
				Before string `json:"before"`
				After  string `json:"after"`
			} `json:"pairs"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("welcomeClips.json: %v", err)
	}
	inJSON := map[string]bool{}
	for _, s := range data.Steps {
		for _, p := range s.Pairs {
			for _, f := range []string{p.Before, p.After} {
				if f == "" {
					t.Errorf("шаг %s, пара %s: пустое имя клипа", s.ID, p.ID)
					continue
				}
				inJSON[f] = true
			}
		}
	}
	embedded := map[string]bool{}
	for _, n := range Names() {
		embedded[n] = true
	}
	for f := range inJSON {
		if !embedded[f] {
			t.Errorf("клип %q есть в welcomeClips.json, но не встроен", f)
		}
	}
	for n := range embedded {
		if !inJSON[n] {
			t.Errorf("встроенный клип %q не упомянут в welcomeClips.json", n)
		}
	}
}

// mpeg1L3Kbps — битрейт MPEG-1 Layer III по индексу из заголовка кадра (0 и 15 — недопустимы).
var mpeg1L3Kbps = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

// firstFrameKbps — битрейт по заголовку первого кадра MPEG: тег ID3v2 (10 байт заголовка +
// размер synchsafe) пропускается; кадр обязан быть MPEG-1 Layer III.
func firstFrameKbps(b []byte) (int, string) {
	off := 0
	if len(b) >= 10 && string(b[:3]) == "ID3" {
		size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
		off = 10 + size
		if b[5]&0x10 != 0 { // флаг footer: ещё 10 байт
			off += 10
		}
	}
	for ; off+4 <= len(b); off++ {
		if b[off] != 0xFF || b[off+1]&0xE0 != 0xE0 {
			continue
		}
		h := b[off+1]
		if (h>>3)&0x3 != 0x3 {
			return 0, "первый кадр не MPEG-1"
		}
		if (h>>1)&0x3 != 0x1 {
			return 0, "первый кадр не Layer III"
		}
		idx := b[off+2] >> 4
		if mpeg1L3Kbps[idx] == 0 {
			return 0, "недопустимый индекс битрейта"
		}
		return mpeg1L3Kbps[idx], ""
	}
	return 0, "кадр MPEG не найден"
}

// ТК3 (internal-welcome-presets): каждый встроенный клип — MPEG-1 Layer III не ниже 192 кбит/с
// по заголовку первого кадра.
func TestClipsBitrateAtLeast192(t *testing.T) {
	t.Parallel()
	names := Names()
	if len(names) == 0 {
		t.Fatal("Names() пуст: встроенных клипов нет")
	}
	for _, n := range names {
		b, err := Clip(n)
		if err != nil {
			t.Errorf("Clip(%q): %v", n, err)
			continue
		}
		kbps, why := firstFrameKbps(b)
		if why != "" {
			t.Errorf("клип %s: %s", n, why)
			continue
		}
		if kbps < 192 {
			t.Errorf("клип %s: битрейт %d кбит/с, want ≥ 192", n, kbps)
		}
	}
}

// Сам разборщик: без проверки на известных байтах тест выше мог бы молча пройти.
func TestFirstFrameKbpsParser(t *testing.T) {
	t.Parallel()
	frame := func(b2 byte) []byte { return []byte{0xFF, 0xFB, b2, 0x00} } // MPEG-1 L3
	id3 := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0x01, 0x00}               // размер synchsafe = 128
	withTag := append(append(append([]byte{}, id3...), make([]byte, 128)...), frame(0x90)...)
	// внутри тега байты, похожие на синхрослово, — разборщик обязан их пропустить
	withTag[12], withTag[13], withTag[14] = 0xFF, 0xFB, 0x10
	cases := []struct {
		name string
		in   []byte
		kbps int
		bad  bool
	}{
		{"192 без тега", frame(0xB0), 192, false},
		{"320 без тега", frame(0xE0), 320, false},
		{"128 после ID3v2", withTag, 128, false},
		{"96", frame(0x70), 96, false},
		{"MPEG-2", []byte{0xFF, 0xF3, 0xB0, 0}, 0, true},
		{"Layer II", []byte{0xFF, 0xFD, 0xB0, 0}, 0, true},
		{"индекс 15", frame(0xF0), 0, true},
		{"пусто", nil, 0, true},
	}
	for _, c := range cases {
		k, why := firstFrameKbps(c.in)
		if c.bad != (why != "") || k != c.kbps {
			t.Errorf("%s: got %d %q, want %d bad=%v", c.name, k, why, c.kbps, c.bad)
		}
	}
}
