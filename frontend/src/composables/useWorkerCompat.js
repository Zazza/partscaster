// Плашка «воркер и приложение разных версий»: версия проверяется, когда воркер стал доступен
// (запуск, смена адреса, перезапуск воркера), а не на каждом опросе статуса.
import { ref } from 'vue'
import { api } from '../api.js'

const warning = ref('')
let checked = false
let generation = 0 // растёт при recheck: ответ, запрошенный до смены адреса, не перетирает новый

// onHealth — вызывать на каждом опросе статуса: h — ответ /health или null (недоступен)
async function onHealth(h) {
  if (!h) { checked = false; return }
  if (checked) return
  checked = true
  const gen = generation
  try {
    const r = await api.workerVersion()
    if (gen === generation) warning.value = r?.warning || ''
  } catch {
    if (gen === generation) checked = false // не ответил на /version сейчас — спросим на следующем опросе
  }
}

function dismiss() { warning.value = '' }

// recheck — адрес воркера сменили в настройках: версия нового воркера проверится на ближайшем опросе
function recheck() { generation++; checked = false; warning.value = '' }

export function useWorkerCompat() {
  return { warning, onHealth, dismiss, recheck }
}
