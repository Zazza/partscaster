//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Чанк 60..120 с доиграл до конца сам (процесс плеера завершился без ошибки):
// позиция — конец трека (120 с), а не начало чанка (60 с). Плеер — заглушка
// pw-play в PATH, которая сразу выходит с кодом 0; выполняется настоящий код
// естественного завершения (spawnLocked → cmd.Wait).
func TestNaturalEndSetsPositionToEnd(t *testing.T) {
	bin := t.TempDir()
	stub := filepath.Join(bin, "pw-play")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	p := &player{volume: 0.8}
	if err := p.Load(1, []byte("RIFF"), 120*time.Second); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Stop)
	p.mu.Lock()
	p.offset = 60 * time.Second // как после перемотки на середину
	p.mu.Unlock()
	if err := p.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		playing, pos, _, _ := p.State()
		if !playing {
			if pos != 120*time.Second {
				t.Fatalf("pos = %v, want 120s (конец трека, а не начало чанка)", pos)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("заглушка плеера не завершилась за 5 с")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
