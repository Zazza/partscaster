//go:build !linux

package main

import "github.com/wailsapp/wails/v2/pkg/options"

// windowBackground — фон окна под webview: вне Linux — по умолчанию Wails (углы не скругляем).
func windowBackground() *options.RGBA { return nil }
