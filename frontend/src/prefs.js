// Запомненные галочки интерфейса (localStorage): '1' — вкл, '0' — выкл. Без localStorage (или он бросает) —
// значение по умолчанию при чтении, тихий пропуск при записи: галочка живёт до закрытия экрана.

/** Галочка key: запомненное значение или def (записи нет, она чужая или хранилища нет). */
export function readFlag(key, def = false, storage = globalThis.localStorage) {
  try {
    const v = storage.getItem(key)
    return v === '1' ? true : v === '0' ? false : def
  } catch {
    return def   // нет localStorage — значение по умолчанию
  }
}

/** Запомнить галочку key. */
export function writeFlag(key, on, storage = globalThis.localStorage) {
  try { storage.setItem(key, on ? '1' : '0') } catch { /* без localStorage — до закрытия экрана */ }
}
