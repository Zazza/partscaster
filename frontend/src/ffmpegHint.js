// Окно «Нужен ffmpeg» по ОС (Environment().platform из Wails): на Windows приложение само качает закреплённую
// сборку, на Linux/macOS — только подсказка системной установки. Неизвестная ОС — как Linux (без кнопки «Скачать»).
export function ffmpegVariant(platform) {
  if (platform === 'windows') return { kind: 'install' }
  if (platform === 'darwin') return { kind: 'hint', hintKey: 'ffmpeg.hint.mac' }
  return { kind: 'hint', hintKey: 'ffmpeg.hint.linux' }
}
