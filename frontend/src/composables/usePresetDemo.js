// Демо-эталоны для прослушивания пресетов звука: на семью жанровых пресетов — короткий
// инструментальный черновик, создаётся приложением по требованию (submit draft с фиксированным
// сидом — один и тот же эталон для всех), папка «Демо» (в списке треков скрыт — jobFilter),
// дорожки — лениво при первом прослушивании. Синглтон: состояние в module scope.
// Кеш id демо переживает перезапуск (localStorage); демо-джоба — обычный трек, её можно удалить —
// следующее ensure создаст новую.
import { ref } from 'vue'
import { api } from '../api.js'
import { DEMO_FOLDER } from '../jobFilter.js'
import { PRESET_FAMILIES } from '../soundPresets.js'

// стили черновиков — английские теги YuE, как у стилевых пресетов библиотеки
export const DEMO_STYLES = {
  'Рок': {
    seed: 910001,
    style: 'Instrumental, no vocals. Indie rock: live drums, driving melodic bass, jangly clean guitars, energetic, 120 BPM.',
  },
  'Тяжёлое': {
    seed: 910002,
    style: 'Instrumental, no vocals. Heavy metal: punchy live drums, down-tuned distorted guitar riffs, deep dense bass, 140 BPM.',
  },
  'Электроника': {
    seed: 910003,
    style: 'Instrumental, no vocals. Synth-pop electronic: four-on-the-floor drum machine, analog synth bass, bright synth pads, 122 BPM.',
  },
  'Поп и другое': {
    seed: 910004,
    style: 'Instrumental, no vocals. Warm pop: soft drums, round bass, clean electric guitar and piano chords, gentle, 100 BPM.',
  },
}

const CACHE_KEY = 'yue_preset_demo'

// статус демо: cooking — генерируется, stems — делаются дорожки, ready — можно слушать,
// error — не получилось (reset/повторный ensure создаст заново)
const demos = ref({})          // {семья: {jobId, status, err, stems: string[], durationSec}}
const inflight = {}            // семья → Promise (ensure/stems не дублируются)
const stemsInflight = {}

const set = (fam, patch) => { demos.value = { ...demos.value, [fam]: { ...(demos.value[fam] || {}), ...patch } } }

function readCache() {
  try { return JSON.parse(localStorage.getItem(CACHE_KEY) || '{}') } catch { return {} }
}
function writeCache(fam, id) {
  const c = readCache()
  c[fam] = id
  try { localStorage.setItem(CACHE_KEY, JSON.stringify(c)) } catch { /* приватный режим — просто без кеша */ }
}
function dropCache(fam) {
  const c = readCache()
  delete c[fam]
  try { localStorage.setItem(CACHE_KEY, JSON.stringify(c)) } catch { /* без кеша */ }
}

// известная семья или первая из PRESET_FAMILIES (у своих пресетов семьи нет)
const famOf = (family) => (DEMO_STYLES[family] ? family : PRESET_FAMILIES[0])

// джоба по id: null — её нет (удалили); сбой связи с воркером — исключение (не повод генерировать демо заново)
async function findJob(id) {
  const jobs = (await api.jobs()) || []
  return jobs.find((j) => j.id === id) || null
}

/** Демо семьи готово к использованию: проверяет кеш и воркер, нет/сломана — создаёт.
 *  Возвращает id демо-джобы; гарантирует единственный параллельный вызов на семью. */
function ensure(family) {
  const fam = famOf(family)
  const cur = demos.value[fam]
  if (cur && ['cooking', 'stems', 'ready'].includes(cur.status) && cur.jobId) return Promise.resolve(cur.jobId)
  if (inflight[fam]) return inflight[fam]
  inflight[fam] = (async () => {
    try {
      let id = (cur && cur.jobId) || readCache()[fam] || 0
      if (id) {
        const job = await findJob(id)
        if (job && job.status === 'done') {
          set(fam, { jobId: id, status: 'ready', err: '', file: job.audio_file || '',
            durationSec: job.duration_sec || cur?.durationSec || 0 })
          return id
        }
        if (job && ['queued', 'running'].includes(job.status)) {
          set(fam, { jobId: id, status: 'cooking', err: '', file: '' })
          return id
        }
        dropCache(fam)   // ошибка, отмена или демо удалили — создаём новую
        id = 0
      }
      const s = DEMO_STYLES[fam]
      set(fam, { status: 'cooking', err: '' })
      const jobId = await api.submit({
        title: `Демо · ${fam}`, lyrics: '[Instrumental]', style: s.style, seed: s.seed, draft: true, cot: 'off',
      })
      writeCache(fam, jobId)
      set(fam, { jobId, status: 'cooking', durationSec: 0, file: '' })
      // папка прячет демо из общего списка; не вышло — демо всё равно работает
      try { await api.setJobFolder(jobId, DEMO_FOLDER) } catch { /* папка не критична */ }
      return jobId
    } catch (e) {
      set(fam, { status: 'error', err: String(e) })
      throw e
    } finally {
      delete inflight[fam]
    }
  })()
  return inflight[fam]
}

/** Продвинуть статусы по списку джоб (поллинг открытого редактора): готово/ошибка. */
async function refresh() {
  const cooking = Object.entries(demos.value).filter(([, v]) => v.status === 'cooking' && v.jobId)
  if (!cooking.length) return
  let jobs
  try { jobs = (await api.jobs()) || [] } catch { return }
  for (const [fam, v] of cooking) {
    const job = jobs.find((x) => x.id === v.jobId)
    if (!job) continue
    if (job.status === 'done') set(fam, { status: 'ready', durationSec: job.duration_sec || 0, err: '', file: job.audio_file || '' })
    else if (job.status === 'error' || job.status === 'canceled') set(fam, { status: 'error', err: job.error || job.status })
  }
}

async function stemNames(jobId) {
  try { return ((await api.jobStems(jobId)) || []).map((s) => s.name) } catch { return [] }
}

/** Дорожки демо (для превью): пусто — разделить makeStems и прочитать снова. Возвращает имена. */
function ensureStems(family) {
  const fam = famOf(family)
  const cur = demos.value[fam]
  if (!cur || !cur.jobId || cur.status !== 'ready') return Promise.resolve([])
  if ((cur.stems || []).length) return Promise.resolve(cur.stems)
  if (stemsInflight[fam]) return stemsInflight[fam]
  stemsInflight[fam] = (async () => {
    try {
      set(fam, { status: 'stems' })
      let names = await stemNames(cur.jobId)
      if (!names.length) {
        await api.makeStems(cur.jobId)
        names = await stemNames(cur.jobId)
      }
      set(fam, { status: 'ready', stems: names })
      return names
    } catch (e) {
      set(fam, { status: 'ready', stems: [], err: String(e) })
      return []
    } finally {
      delete stemsInflight[fam]
    }
  })()
  return stemsInflight[fam]
}

/** Имена дорожек демо (Set для previewSource); дорожек нет — пустой Set. */
function available(family) {
  const cur = demos.value[famOf(family)]
  return new Set((cur && cur.stems) || [])
}

/** Дождаться готовности демо (поллинг списка джоб): 'ready' | 'error' | 'timeout' — применение
 *  пресета к недоготовленному демо бессмысленно, звука ещё нет. */
// cancelled() → true — ожидание прекращается (окно редактора закрыли): опрос воркера не идёт ещё 10 минут
function waitReady(family, timeoutSec = 600, cancelled = () => false) {
  const fam = famOf(family)
  return new Promise((resolve) => {
    const started = Date.now()
    const tick = async () => {
      if (cancelled()) return resolve('cancelled')
      try {
        await ensure(fam)
        if (cancelled()) return resolve('cancelled')   // закрыли во время ensure — без лишнего опроса
        await refresh()
      } catch { /* ошибка уже в статусе семьи */ }
      if (cancelled()) return resolve('cancelled')
      const st = (demos.value[fam] || {}).status
      if (st === 'ready') return resolve('ready')
      if (st === 'error') return resolve('error')
      if (Date.now() - started > timeoutSec * 1000) return resolve('timeout')
      setTimeout(tick, 5000)
    }
    tick()
  })
}

/** Забыть демо семьи (ошибка джобы): следующее ensure создаст новую. */
function reset(family) {
  const fam = famOf(family)
  dropCache(fam)
  delete inflight[fam]
  delete stemsInflight[fam]
  demos.value = { ...demos.value, [fam]: { status: 'idle', stems: [] } }
}

export function usePresetDemo() {
  return { DEMO_FOLDER, demos, ensure, refresh, ensureStems, available, reset, waitReady }
}
