// Окно правок студии (сверху справа): подпись «N правок» и запомненное «раскрыто/свёрнуто». Чистая логика без DOM.
import { readFlag, writeFlag } from './prefs.js'

const KEY = 'studio.editsPanel.open'

// «0 правок / 1 правка / 2 правки / 5 правок / 21 правка»; en — «0 edits / 1 edit / 2 edits»
export function editsCountLabel(n, lang) {
  const k = Math.abs(Math.trunc(Number(n) || 0))
  if (lang === 'en') return `${k} ${k === 1 ? 'edit' : 'edits'}`
  const d = k % 10
  const dd = k % 100
  const word = d === 1 && dd !== 11 ? 'правка' : d >= 2 && d <= 4 && (dd < 12 || dd > 14) ? 'правки' : 'правок'
  return `${k} ${word}`
}

// раскрыто ли окно: по умолчанию (и без хранилища) — свёрнуто
export function loadPanelOpen(storage = globalThis.localStorage) {
  return readFlag(KEY, false, storage)
}

export function savePanelOpen(open, storage = globalThis.localStorage) {
  writeFlag(KEY, open, storage)
}
