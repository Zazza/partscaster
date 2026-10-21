package main

import "partscaster/internal/ffmpegbin"

// ffmpeg: перемотка плеера и эффекты студии запускают его локально. На чистой Windows его нет —
// фронтенд при старте спрашивает YueFfmpegStatus и, если пусто, показывает окно установки.

func (a *App) YueFfmpegStatus() ffmpegbin.Status     { return ffmpegbin.Get() }
func (a *App) YueFfmpegInstall() error               { return ffmpegbin.Install() }
func (a *App) YueFfmpegProgress() ffmpegbin.Progress { return ffmpegbin.Snapshot() }
