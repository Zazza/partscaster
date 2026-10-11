// Доска педалей и её дорожка — на трек, переживают перезапуск (localStorage 'yue_studio_state', ключ pedals[jobId]).
// Активная дорожка студии (active) задаёт дорожку доски, если педали на неё ставятся.
import { ref, watch } from 'vue'

const STATE_KEY = 'yue_studio_state'

/** jobId — геттер id трека; active — геттер активной дорожки студии (null — не задана); stems — дорожки педалей. */
export function usePedalBoardState(jobId, active, stems) {
  const board = ref([])
  const stem = ref('guitar')

  function load() {
    try {
      const st = JSON.parse(localStorage.getItem(STATE_KEY) || '{}')
      const per = (st.pedals && st.pedals[jobId()]) || {}
      if (Array.isArray(per.board)) board.value = per.board
      if (typeof per.stem === 'string') stem.value = per.stem
    } catch { /* битое состояние — начинаем с пустой доски */ }
  }
  function save() {
    try {
      const st = JSON.parse(localStorage.getItem(STATE_KEY) || '{}')
      st.pedals = st.pedals || {}
      st.pedals[jobId()] = { board: board.value, stem: stem.value }
      localStorage.setItem(STATE_KEY, JSON.stringify(st))
    } catch { /* нет localStorage — доска живёт до закрытия студии */ }
  }

  load()   // сохранённая доска — до слежения: watch активной дорожки (immediate) иначе сохранил бы пустую доску поверх
  watch([board, stem], save, { deep: true })
  watch(active, (a) => { if (a !== null && stems.includes(a) && a !== stem.value) stem.value = a }, { immediate: true })

  return { board, stem }
}
