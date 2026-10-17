package main

import "github.com/wailsapp/wails/v2/pkg/options"

// windowBackground — фон окна под webview. На Linux прозрачный: окно полупрозрачное
// (linux.Options.WindowIsTranslucent), скругление углов рисует CSS (TitleBar.vue).
func windowBackground() *options.RGBA { return &options.RGBA{} }
