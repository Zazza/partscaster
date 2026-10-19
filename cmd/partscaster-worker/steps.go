package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"partscaster/internal/workerexe"
)

// realSteps — шаги установки на этой машине (workerexe.InstallSteps). Каждый пропускает сделанное: «Повторить» и
// следующий запуск продолжают с места, скачанное не перекачивается.
type realSteps struct {
	home, workerDir string
	model           string
	lowVram         bool
}

// Packages — Python 3.12 и пакеты воркера (первый раз или сменился набор).
func (s *realSteps) Packages(_ context.Context, out io.Writer) error {
	reqPath := filepath.Join(s.workerDir, "requirements.txt")
	req, err := os.ReadFile(reqPath)
	if err != nil {
		return err
	}
	if !workerexe.NeedInstall(s.home, req) {
		sayln(out, "Python и пакеты воркера уже установлены")
		return nil
	}
	sayln(out, "Установка Python и пакетов (первый раз — несколько гигабайт)…")
	uv, err := workerexe.EnsureUv(s.home)
	if err != nil {
		return fmt.Errorf("uv: %w", err)
	}
	if err := workerexe.InstallPackages(s.home, uv, reqPath, out); err != nil {
		return fmt.Errorf("пакеты не встали: %w", err)
	}
	return nil
}

// Sep — разделение на дорожки (RoFormer): своё окружение и веса обеих моделей.
func (s *realSteps) Sep(ctx context.Context, out io.Writer) error {
	if workerexe.NeedSepInstall(s.home) {
		sayln(out, "Установка разделения на дорожки (RoFormer, первый раз — несколько гигабайт)…")
		uv, err := workerexe.EnsureUv(s.home)
		if err != nil {
			return fmt.Errorf("uv: %w", err)
		}
		if err := workerexe.InstallSep(s.home, uv, out); err != nil {
			return fmt.Errorf("разделение на дорожки не встало: %w", err)
		}
	}
	if err := runCmd(ctx, workerexe.SepDownloadCommand(s.home, s.workerDir), out); err != nil {
		return fmt.Errorf("веса разделения на дорожки не скачались: %w", err)
	}
	return nil
}

// Model — модель генерации yue2.cpp: скачанная выбранная не перекачивается; настройки воркера — выбор окна.
func (s *realSteps) Model(ctx context.Context, out io.Writer) error {
	if workerexe.CppBinary(s.workerDir, "windows") == "" {
		return errors.New("в этом пакете нет программы yue2.cpp (движок генерации) — скачайте пакет Windows заново")
	}
	installed := runCmd(ctx, workerexe.ModelInstalledCommand(s.home, s.workerDir, s.model), io.Discard) == nil
	if installed {
		say(out, "Модель %s уже скачана\n", strings.ToUpper(s.model))
	} else {
		say(out, "Скачивание модели %s (несколько гигабайт — зависит от сети)…\n", strings.ToUpper(s.model))
		if err := runCmd(ctx, workerexe.DownloadEngineModel(s.home, s.workerDir, s.model), out); err != nil {
			return fmt.Errorf("модель %s не скачалась: %w", s.model, err)
		}
	}
	// engine: nil — выбор движка прежних версий убирается: движок один
	return workerexe.WriteSettings(s.home, map[string]any{"cpp_model": s.model, "cpp_low_vram": s.lowVram, "engine": nil})
}

// say/sayln — строка в журнал/окно установки: сбой записи журнала не останавливает установку и воркер.
func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
func sayln(w io.Writer, args ...any)              { _, _ = fmt.Fprintln(w, args...) }

// runCmd — процесс с выводом в out; отмена ctx завершает его.
func runCmd(ctx context.Context, cmd *exec.Cmd, out io.Writer) error {
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill() // прерывание установки: процесс уже не нужен
		<-done
		return ctx.Err()
	}
}
