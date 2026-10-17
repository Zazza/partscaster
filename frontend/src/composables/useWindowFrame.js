// Своя рамка окна (main.go: Frameless): заголовок, состояние «развёрнуто» и поправка
// края растягивания. Синглтон: состояние в области модуля, как usePlayer.
import { ref, watch } from 'vue'
import { YueSetWindowTitle } from '../wailsjs/go/main/App'
import { WindowIsMaximised, Environment } from '../wailsjs/runtime/runtime.js'
import { frameEdge } from '../windowEdge.js'

// заголовок в полосе окна; пусто — полоса показывает название приложения
export const windowTitle = ref('')
export const maximised = ref(false)

// заголовок и в полосе, и системный (панель задач, alt-tab)
export function setWindowTitle(title) {
  windowTitle.value = title
  return YueSetWindowTitle(title)
}

async function syncMaximised() {
  try { maximised.value = await WindowIsMaximised() } catch { /* не wails — окна нет */ }
}

async function syncPlatform() {
  try {
    const env = await Environment()
    document.documentElement.classList.toggle('win-rounded', env.platform === 'linux')
  } catch { /* не wails — окна нет */ }
}

let installed = false
// Слушатели окна. Рантайм Wails вешает свой mousemove на window раньше (его скрипт
// вставляется в head до модуля приложения), поэтому здесь его выбор края уже сделан —
// поправляем флаг и курсор. window.wails.flags — внутренности Wails v2.
export function installWindowFrame() {
  if (installed) return
  installed = true
  // развёрнутое окно — без рамки (класс на корне, стили в TitleBar.vue)
  watch(maximised, (v) => document.documentElement.classList.toggle('win-maximised', v))
  syncMaximised()
  // скруглённые углы — только на Linux: там под ними прозрачный фон окна (frame_linux.go)
  syncPlatform()
  window.addEventListener('resize', syncMaximised)
  window.addEventListener('mousemove', () => {
    const flags = window.wails?.flags
    if (!flags?.enableResize) return
    const edge = frameEdge(flags.resizeEdge, maximised.value)
    if (edge === flags.resizeEdge) return
    flags.resizeEdge = edge
    document.documentElement.style.cursor = edge || flags.defaultCursor || ''
  })
}
