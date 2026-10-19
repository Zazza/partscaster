package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ТК6: Load называет временный файл по содержимому: mp3 (тег «ID3» или кадр 0xFF 0xFB) →
// .mp3, «fLaC» → .flac, прочее → .wav. После Stop временный файл удалён.
func TestLoadExtByContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []byte
		ext  string
	}{
		{"mp3 с тегом ID3", append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), make([]byte, 64)...), ".mp3"},
		{"mp3 без тега, кадр 0xFF 0xFB", append([]byte{0xFF, 0xFB, 0x90, 0x64}, make([]byte, 64)...), ".mp3"},
		{"flac", append([]byte("fLaC\x00\x00\x00\x22"), make([]byte, 64)...), ".flac"},
		{"wav RIFF", append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), make([]byte, 64)...), ".wav"},
		{"прочие байты", []byte("hello, not audio at all"), ".wav"},
		{"короче заголовка", []byte{0xFF}, ".wav"},
		{"пусто", nil, ".wav"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := &player{volume: 0.8}
			if err := p.Load(42, c.data, time.Second); err != nil {
				t.Fatalf("Load: %v", err)
			}
			file := p.tmpFile
			t.Cleanup(func() { os.Remove(file) })
			if !strings.HasSuffix(file, c.ext) {
				t.Errorf("временный файл %q, want суффикс %s", file, c.ext)
			}
			if _, err := os.Stat(file); err != nil {
				t.Errorf("временный файл не создан: %v", err)
			}
			p.Stop()
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Errorf("после Stop временный файл %q не удалён (err=%v)", file, err)
			}
		})
	}
}
