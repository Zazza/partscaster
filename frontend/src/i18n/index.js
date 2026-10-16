// Единое состояние языка интерфейса: один инстанс на приложение,
// словари в ru.js/en.js (плоские ключи), язык — в localStorage.
import { ref } from 'vue'
import RU from './ru.js'
import EN from './en.js'

const dicts = { ru: RU, en: EN }
// хранилище может отсутствовать (node в тестах чистых модулей) или бросать — тогда русский по умолчанию
function storedLocale() {
  try { return globalThis.localStorage?.getItem('yue_lang') || 'ru' } catch { return 'ru' }
}
const locale = ref(storedLocale())

/** Перевод по ключу; {имя} подставляется из vars. Нет ключа — сам ключ. */
function t(key, vars) {
  let s = dicts[locale.value][key] ?? dicts.ru[key] ?? key
  if (vars) {
    for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v))
  }
  return s
}

function setLocale(l) {
  if (!dicts[l]) return
  locale.value = l
  try { localStorage.setItem('yue_lang', l) } catch { /* без хранилища язык живёт до перезапуска */ }
  if (typeof document !== 'undefined') document.documentElement.setAttribute('lang', l)
}

export function useI18n() {
  return { locale, t, setLocale }
}
