<script setup>
// Partscaster — корневой компонент: форма новой композиции, очередь/результаты,
// переключение страниц. Экранные компоненты и модалки — в ./components/.
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { api } from './api.js'
import { rackCompile } from './rack.js'
import { groups as builtinGroups, loadCustomGroups, saveCustomGroups } from './groups.js'
import { slotKeys, slotHints, slotHintsEn, durOptions } from './slotOptions.js'
import { CHARACTER, characterPayload, characterLabel } from './character.js'
import { useI18n } from './i18n/index.js'
import { voiceDescriptor, normalizeVoiceParams } from './voiceLab.js'
import { defaultJobFilter, filterJobs, groupJobs, listMore, LIST_STEP, folderNames, FOLDER_NONE, activeFilterCount } from './jobFilter.js'
import { styleRows, styleSummary } from './styleTags.js'
import { isResultJob, mixChildId, mixLabel } from './insertLabels.js'
import { useInserts } from './composables/useInserts.js'
import { useWorkerCompat } from './composables/useWorkerCompat.js'
// сервис «перепеть с места»: живёт всё время, как вклейки (студию закрывают)
import './composables/useRevoice.js'
import { buildStyleLine, cleanLyrics, withInstrumental, instrumentalLine } from './styleLogic.js'
import { formStyle, hasCyrillic, newTrackPayload } from './requests.js'
import { insertSectionTag, SECTION_TAGS } from './lyricsTags.js'
import { usePlayer, fmtDur } from './composables/usePlayer.js'
import { useConfirm } from './composables/useConfirm.js'
import VSelect from './VSelect.vue'
import PlayerBar from './components/PlayerBar.vue'
import SideBar from './components/SideBar.vue'
import SettingsPage from './components/SettingsPage.vue'
import MyStylesModal from './components/MyStylesModal.vue'
import VoicesPage from './components/VoicesPage.vue'
import InstrumentsPage from './components/InstrumentsPage.vue'
import PresetsPage from './components/PresetsPage.vue'
import ReplayModal from './components/ReplayModal.vue'
import RecipeImportModal from './components/RecipeImportModal.vue'
import { activeImports } from './recipe.js'
import { engineLabel, retryWarning, currentEngineLabel } from './engine.js'
import PresetChips from './components/PresetChips.vue'
import JobPresets from './components/JobPresets.vue'
import StudioPage from './components/StudioPage.vue'
import WelcomeModal from './components/WelcomeModal.vue'
import { WELCOME_KEY, welcomeTab, welcomeClosed } from './welcome.js'
import { RELEASE_NOTES } from './releaseNotes.js'
import logoUrl from './assets/logo.png'
import ConfirmModal from './components/ConfirmModal.vue'
import TitleBar from './components/TitleBar.vue'
import { keyAction } from './hotkeys.js'
import { fanCount, FAN_DEFAULT } from './fanCount.js'
import PageView from './components/PageView.vue'
import PageBox from './components/PageBox.vue'
import RackPicker from './components/RackPicker.vue'
import MetricsModal from './components/MetricsModal.vue'
import PlanModal from './components/PlanModal.vue'

const serverURL = ref('')

// тема: тёмная по умолчанию, светлая — [data-theme="light"]
const theme = ref(localStorage.getItem('yue_theme') || 'dark')
document.documentElement.setAttribute('data-theme', theme.value)
function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  localStorage.setItem('yue_theme', theme.value)
  document.documentElement.setAttribute('data-theme', theme.value)
}

// размер окна: восстанавливаем сохранённый при запуске, изменения пишем с дебаунсом
// (вне wails — в браузере/тестах — рантайма нет, тихо пропускаем)
import { WindowGetSize, WindowSetSize } from './wailsjs/runtime/runtime.js'
try {
  const saved = JSON.parse(localStorage.getItem('yue_window') || 'null')
  if (saved && saved.w >= 900 && saved.h >= 600) WindowSetSize(saved.w, saved.h)
} catch { /* первый запуск или повреждённая запись — дефолт из main.go */ }
let winSizeTimer = 0
window.addEventListener('resize', () => {
  clearTimeout(winSizeTimer)
  winSizeTimer = setTimeout(async () => {
    try {
      const s = await WindowGetSize()
      if (s.w >= 900 && s.h >= 600) localStorage.setItem('yue_window', JSON.stringify({ w: s.w, h: s.h }))
    } catch { /* не wails — сохранять нечего */ }
  }, 700)
})

const { playerState, playBusy, isPlaying, toggleArtifact, onVolume, onRefresh } = usePlayer()
const { locale, t, setLocale } = useI18n()
const healthTitle = computed(() => health.value
  ? t('app.health.up') + (health.value.model_loaded ? t('app.health.model') : '')
  : t('app.health.down'))
const statusLabelC = computed(() => ({
  queued: t('queue.status.queued'), running: t('queue.status.running'), done: t('queue.status.done'),
  error: t('queue.status.error'), canceled: t('queue.status.canceled'),
}))
const styleTagLabel = (slot) => t('style.tag.' + slot)
const { askConfirm } = useConfirm()

const slots = ref({
  language: 'Russian',
  genre: '', rhythm: '', guitars: '', keys: '', vocals: '', mood: '', production: '',
  bpm: null,
})
const styleOverride = ref('')
const lyrics = ref(`[Verse]\n...\n\n[Chorus]\n...`)
const seed = ref(null)
// пресеты звука для нового трека (до 3) и названия пресетов для строк в карточках
const soundPresetIds = ref([])
const presetNames = ref({})
const presetAsked = new Set()   // id, за которыми уже ходили (удалённый пресет не ищем снова)
async function loadPresetNames() {
  try {
    presetNames.value = Object.fromEntries(((await api.soundPresets()) || []).map((p) => [p.id, p.name]))
  } catch { /* старый воркер — строки покажут номер пресета */ }
}
const cot = ref('full')
const arcKind = ref('')  // драматургия: '' | build | wave | burst
// характер исполнения: смелость игры (температура) и точность по стилю/нотам (cfg)
const temperature = ref(CHARACTER.temperature.def)
const cfgScale = ref(CHARACTER.cfg.def)
const characterDefault = computed(() => !Object.keys(characterPayload(temperature.value, cfgScale.value)).length)
// «Дополнительно» свёрнуто — изменённое показывается в его заголовке
const advancedSummary = computed(() => {
  const parts = []
  if (seed.value) parts.push('seed ' + seed.value)
  if (cot.value !== 'full') parts.push(t('form.cot.' + cot.value))
  if (arcKind.value) parts.push(t('arc.' + arcKind.value))
  const ch = characterPayload(temperature.value, cfgScale.value)
  const chl = characterLabel(ch)
  if (chl) parts.push(chl)
  return parts.join(' · ')
})
function characterReset() {
  temperature.value = CHARACTER.temperature.def
  cfgScale.value = CHARACTER.cfg.def
}
// «без слов»: стих не нужен, на воркер уйдёт заглушка [Instrumental]; голос в слотах стиля → «без голоса»,
// выключили — голос стиля возвращается (keptVocals)
const noLyrics = ref(false)
let keptVocals = ''
function applyInstrumental(on) {
  const r = withInstrumental(slots.value, on, keptVocals)
  slots.value = r.slots
  keptVocals = r.kept
}
watch(noLyrics, applyInstrumental)
// длительность инструментала: длина трека у YuE2 задаётся числом секций в тексте,
// явного параметра нет; диапазон размножает [Instrumental]-секции
const durMode = ref('auto')
const title = ref('')
const submitting = ref(false)

// инструментальная стойка: [{id, effect}] — разворачивается в текст стиля
const rackSel = ref([])   // инструменты: [{ id, effect }] (RackPicker)

// свой выпадающий список подсказок слотов (нативные datalist в WebView2 глючат)
const openSlot = ref('')
// подсказки: весь список без обрезки (обрезка в 30 прятала хвост алфавита — металл
// и прочее в конце списка было не найти), подписи и поиск — на языке интерфейса
function slotFiltered(key) {
  const v = (slots.value[key] || '').trim().toLowerCase()
  const en = locale.value === 'en'
  const opts = (en ? slotHintsEn : slotHints)[key] || []
  const label = (o) => (en ? o[1] : o[0])
  return v ? opts.filter((o) => label(o).toLowerCase().includes(v)) : opts
}

const compiledStyle = computed(() =>
  styleOverride.value.trim() || buildStyleLine(slots.value, rackCompile(rackSel.value)))

// поле «Стиль одной строкой»: пока не трогали — показывает то, что собрано
// из слотов и стойки; первая правка фиксирует ручную версию (переопределяет форму)
const styleLine = computed({
  get: () => styleOverride.value || compiledStyle.value,
  set: (v) => { styleOverride.value = v },
})

// финальная строка стиля к отправке: русские значения из списков подставляются словарём,
// самописная кириллица уходит как есть — под строкой стиля предупреждение (hasCyrillic)
// «без слов» у строки 1:1 — пометка в начале строки (у слотов — голос стиля, см. applyInstrumental)
const sentStyle = computed(() =>
  formStyle(instrumentalLine(styleOverride.value, noLyrics.value), slots.value, rackCompile(rackSel.value)))
function finalStyle() { return sentStyle.value }

function payload(extra = {}) {
  return newTrackPayload({
    title: title.value, lyrics: lyrics.value, noLyrics: noLyrics.value, durMode: durMode.value,
    seed: seed.value, cot: cot.value, arc: arcKind.value, abc: customAbc.value,
    temperature: temperature.value, cfg: cfgScale.value, soundPresetIds: soundPresetIds.value,
  }, extra)
}

// guard отправки: стиль есть и стих есть (или инструментал)
const canSubmit = computed(() =>
  !!compiledStyle.value && (!!lyrics.value.trim() || noLyrics.value))

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  submitErr.value = ''
  try {
    const id = await api.submit({ ...payload(), style: finalStyle() })
    if (customAbc.value.trim()) inheritTrickMarks(id)
    newTrackPage.value = false   // к списку: трек уже в очереди
    await refresh()
  } catch (e) {
    submitErr.value = String(e?.message || e)
  } finally { submitting.value = false }
}

// черновик ~40 с: быстро послушать стиль, прежде чем рендерить полный трек
async function submitDraft() {
  if (!canSubmit.value) return
  submitting.value = true
  submitErr.value = ''
  try {
    const id = await api.submit({ ...payload({ draft: true }), style: finalStyle() })
    if (customAbc.value.trim()) inheritTrickMarks(id)
    newTrackPage.value = false   // к списку: трек уже в очереди
    await refresh()
  } catch (e) {
    submitErr.value = String(e?.message || e)
  } finally { submitting.value = false }
}

async function submitFan(n) {
  if (!canSubmit.value) return
  submitting.value = true
  submitErr.value = ''
  try {
    await api.submitFan({ ...payload(), style: finalStyle() }, n)
    newTrackPage.value = false   // к списку: трек уже в очереди
    await refresh()
  } catch (e) {
    submitErr.value = String(e?.message || e)
  } finally { submitting.value = false }
}

function applyPreset(p, mode) {
  title.value = p.name
  if (p.lyrics !== undefined) lyrics.value = p.lyrics
  if (p.lyrics === '[Instrumental]') noLyrics.value = true
  else if (p.lyrics !== undefined) noLyrics.value = false
  if (p.seed !== undefined) seed.value = p.seed
  cot.value = 'full'
  slots.value = {
    language: '', genre: '', rhythm: '', guitars: '', keys: '',
    vocals: '', mood: '', production: '', bpm: p.bpm || null,
  }
  if (mode === 'line' || !p.slots) {
    // точная строка (1:1) или свой стиль библиотеки — у него нет слотов
    styleOverride.value = p.style
  } else {
    styleOverride.value = ''
    for (const k of Object.keys(p.slots)) slots.value[k] = p.slots[k]
  }
  // голос нового стиля: «без слов» уже включено — сразу без голоса; запомненный голос прежнего стиля — забыть
  keptVocals = ''
  if (noLyrics.value) applyInstrumental(true)
}

// «сохранить стиль» — модалка имени; пустое имя — по названию трека
const styleSaveOpen = ref(false)
const styleSaveName = ref('')
const myStylesOpen = ref(false)
function openStyleSave() {
  styleSaveName.value = (title.value || '').trim()
  styleSaveOpen.value = true
}
// текущая форма → свои стили: строка (или слоты+BPM) как есть
function saveStyleToLibrary() {
  const gid = 'c-from-form'
  if (!customGroups.value.find((g) => g.id === gid)) {
    customGroups.value = [...customGroups.value, { id: gid, name: t('style.fromForm'), items: [] }]
  }
  const g = customGroups.value.find((x) => x.id === gid)
  const name = styleSaveName.value.trim() || t('style.autoName') + ' ' + new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
  const item = {
    id: 'c-' + Date.now().toString(36),
    name,
    style: styleOverride.value.trim() || compiledStyle.value,
  }
  if (!styleOverride.value.trim()) item.slots = { ...slots.value }   // раскладка по слотам сохранится
  const dup = g.items.find((i) => i.name === name)
  g.items = dup ? g.items.map((i) => (i.name === name ? item : i)) : [...g.items, item]
  saveCustomGroups(customGroups.value)
  libGroup.value = gid
  libStyle.value = item.id
}

// библиотека стилей на главной: группы → стили (двухуровневый комбобокс)
const customGroups = ref(loadCustomGroups())
const allGroups = computed(() => [...builtinGroups, ...customGroups.value])
const libGroup = ref('')
const libStyle = ref('')
const groupItems = computed(() => (allGroups.value.find((g) => g.id === libGroup.value) || { items: [] }).items)
const currentItem = computed(() => groupItems.value.find((i) => i.id === libStyle.value))
// имя из библиотеки/стойки на языке интерфейса (свои стили без перевода — как есть)
const dataName = (o) => (locale.value === 'en' ? (o?.nameEn || o?.name) : o?.name)
// группы библиотеки — по алфавиту текущего языка; было в порядке массива, искать было трудно
const libGroupOptions = computed(() => allGroups.value
  .map((g) => ({ value: g.id, label: `${dataName(g)} (${(g.items || []).length})` }))
  .sort((a, b) => a.label.localeCompare(b.label, locale.value === 'en' ? 'en' : 'ru')))
const libStyleOptions = computed(() => groupItems.value.map((i) => ({ value: i.id, label: dataName(i) })))

function onLibGroupChange() { libStyle.value = '' }
function onLibStyleChange() { if (currentItem.value) applyPreset(currentItem.value, 'slots') }
function onLibExact() { if (currentItem.value) applyPreset(currentItem.value, 'line') }

const jobs = ref([])
const health = ref(null)
const compat = useWorkerCompat()   // плашка «воркер/приложение разных версий»
const stats = ref(null)
const procQueue = ref({ paused: false, items: [] })   // общая очередь воркера
let timer = null

const qf = ref(defaultJobFilter())
// производные треки (куски для вклеек, пересборки, варианты) — под родителем
const grouped = computed(() => groupJobs(jobs.value))
const filteredJobs = computed(() => filterJobs(grouped.value.top, qf.value, Date.now(), grouped.value.children))
const openJobs = ref(new Set())      // id треков с развёрнутыми строками (сколько угодно)
const folderPick = ref(null)         // id трека, у которого сейчас открыт селект смены папки
function toggleOpenJob(id) {
  const s = new Set(openJobs.value)
  if (s.has(id)) s.delete(id)
  else { s.add(id); loadKidMixes(id) }   // миксы-вклейки — сразу в общий список вложений
  openJobs.value = s
}
// открытые треки на текущей странице: «свернуть все» и приглушение закрытых строк
const openOnPage = computed(() => queuePage.value.filter((j) => openJobs.value.has(j.id)).length)
function collapseAllJobs() { openJobs.value = new Set() }
function closeMenu(e) {
  const d = e.target.closest('details')
  if (d) d.open = false
}
function closeMenusOutside(e) {
  for (const d of document.querySelectorAll('details.menu-pop[open]')) if (!d.contains(e.target)) d.open = false
}
onMounted(() => document.addEventListener('click', closeMenusOutside))
onUnmounted(() => document.removeEventListener('click', closeMenusOutside))
// готовые миксы с вклейками (файлы эффектов родителя overdub-inst-*) — по раскрытию
const kidMixes = ref({})
const insertsSvc = useInserts()
// подгрузка миксов-вклеек (DSP-варианты с вложениями) для списка вложений
async function loadKidMixes(id) {
  try {
    const vs = (await api.dspVariants(id)) || []
    // свежий первым: текущий результат — последний микс (в нём все вклейки)
    const mixes = vs.filter((v) => mixChildId(v.file) != null)
      .sort((a, b) => String(b.created_at || '').localeCompare(String(a.created_at || '')))
    kidMixes.value = { ...kidMixes.value, [id]: mixes }
  } catch { /* варианты недоступны — покажем только треки-вложения */ }
}
// что под «версии»: дочерние треки и готовые миксы (у трека может быть только микс —
// эффект на дорожку без дочерних треков)
const kidCount = (j) => (grouped.value.children[j.id] || []).length + (j.mixes || 0)
const kidResults = (id) => (grouped.value.children[id] || []).filter(isResultJob)
// незаконченная работа в студии: черновик правок (на воркере) у трека или любой его версии
const songDraft = (j) => !!j.has_draft || (grouped.value.children[j.id] || []).some((k) => k.has_draft)
// основная версия песни (head_id у корня): её играет «▶» и открывает студия
function headOf(j) {
  if (!j.head_id) return j
  return (grouped.value.children[j.id] || []).find((k) => k.id === j.head_id) || j
}
async function makeHead(root, v) {
  try {
    await api.setHead(root.id, v.id === root.id ? 0 : v.id)
    await refresh()
  } catch (e) {
    alert(String(e))
  }
}
// подпись и папка песни: своё название вместо номера, папки «Альбом/Основы/…»
const titleEdit = ref(null) // { id, value } — трек, чьё название правится
function startRename(j) { titleEdit.value = { id: j.id, value: j.title || '' } }
async function saveRename(j) {
  const e = titleEdit.value
  titleEdit.value = null
  if (!e || e.id !== j.id || !e.value.trim() || e.value.trim() === j.title) return
  try {
    const upd = await api.renameJob(j.id, e.value.trim())
    j.title = upd.title
  } catch (err) {
    alert(String(err))
  }
}
async function moveToFolder(j, folder) {
  try {
    const upd = await api.setJobFolder(j.id, folder === FOLDER_NONE ? '' : folder)
    j.folder = upd.folder || ''
  } catch (err) {
    alert(String(err))
  }
}
// песня «играет», если играет любая её версия или материал
const songPlaying = (j) => [j, ...(grouped.value.children[j.id] || [])].some((v) => isPlaying('m' + v.id))
const folderList = computed(() => folderNames(grouped.value.top))
const qFolderOptions = computed(() => [
  { value: 'all', label: t('queue.folder.all') },
  ...folderList.value.map((f) => ({ value: f, label: f, icon: 'folder' })),
  { value: FOLDER_NONE, label: t('queue.folder.none') },
])
const FOLDER_NEW = '\u0000new'
const jobFolderOptions = computed(() => [
  { value: FOLDER_NONE, label: t('queue.folder.none') },
  ...folderList.value.map((f) => ({ value: f, label: f, icon: 'folder' })),
  { value: FOLDER_NEW, label: t('queue.folder.new'), icon: 'plus' },
])
// своя папка: выбор «новая папка…» открывает поле имени у этой песни
const folderNew = ref(null) // { id, value }
function pickFolder(j, v) {
  if (v === FOLDER_NEW) { folderNew.value = { id: j.id, value: '' }; return }
  moveToFolder(j, v)
}
async function saveNewFolder(j) {
  const e = folderNew.value
  folderNew.value = null
  if (e && e.id === j.id && e.value.trim()) await moveToFolder(j, e.value.trim())
}
// время создания: «2026-10-01T17:26:05» → «01.10 17:26» (год — если не текущий)
function fmtWhen(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})/.exec(String(s || ''))
  if (!m) return ''
  const year = m[1] === String(new Date().getFullYear()) ? '' : '.' + m[1]
  return `${m[3]}.${m[2]}${year} ${m[4]}:${m[5]}`
}
const kidMaterial = (id) => (grouped.value.children[id] || []).filter((k) => !isResultJob(k))
function mixKey(parentId, v) { return `mix${parentId}:${v.file}` }
// подпись микса: по реестру вклеек, иначе сохранённая воркером (микс из MCP)
function mixName(parentId, v) {
  const label = mixLabel(v.file, { applied: insertsSvc.appliedFor(parentId).filter((it) => !it.off), jobs: jobs.value,
    labelOf: (id) => t('studio.trick.inst.' + id), fmt: fmtDur })
  if (label) return t('studio.trick.inst.mix', { what: label })
  return v.label || t('studio.trick.inst.variant', { id: mixChildId(v.file) })
}
function playMix(parent, v) {
  toggleArtifact(mixKey(parent.id, v), mixName(parent.id, v),
    () => api.playFile(parent.id, v.file, parent.duration_sec))
}
// догрузка порциями: видны первые listShown найденных; метка в конце списка (listEnd) при подходе
// прокрутки прибавляет ещё LIST_STEP; новый поиск/фильтр — снова с начала
const listShown = ref(LIST_STEP)
const queuePage = computed(() => filteredJobs.value.slice(0, listShown.value))
watch(qf, () => { listShown.value = LIST_STEP }, { deep: true })
const listEnd = ref(null)
let listObserver = null
watch(listEnd, (el) => {
  if (listObserver) { listObserver.disconnect(); listObserver = null }
  if (!el || typeof window === 'undefined' || !window.IntersectionObserver) return
  listObserver = new window.IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) listShown.value = listMore(listShown.value, filteredJobs.value.length)
  }, { rootMargin: '600px' })
  listObserver.observe(el)
})
// метка всё ещё в окне после догрузки (высокое окно) — наблюдатель сам не сработает: переподписка
// заново сообщает текущее пересечение
watch(listShown, () => { if (listObserver && listEnd.value) { listObserver.unobserve(listEnd.value); listObserver.observe(listEnd.value) } })
onUnmounted(() => listObserver && listObserver.disconnect())

const qStatusOptions = computed(() => [
  { value: 'all', label: t('queue.filter.status.all') },
  { value: 'active', label: t('queue.filter.status.active') },
  { value: 'done', label: t('queue.filter.status.done') },
  { value: 'failed', label: t('queue.filter.status.failed') },
])
const qPeriodOptions = computed(() => [
  { value: 'all', label: t('queue.filter.period.all') },
  { value: 'today', label: t('queue.filter.period.today') },
  { value: 'week', label: t('queue.filter.period.week') },
  { value: 'month', label: t('queue.filter.period.month') },
])
const qDurOptions = computed(() => [
  { value: 'all', label: t('queue.filter.dur.all') },
  { value: 'draft', label: t('queue.filter.dur.draft') },
  { value: 'short', label: t('queue.filter.dur.short') },
  { value: 'mid', label: t('queue.filter.dur.mid') },
  { value: 'long', label: t('queue.filter.dur.long') },
])
const qDraftOptions = computed(() => [
  { value: 'all', label: t('queue.filter.draft.all') },
  { value: 'only', label: t('queue.filter.draft.only') },
  { value: 'hide', label: t('queue.filter.draft.hide') },
])


async function refresh() {
  try {
    // очередь — не повод считать воркер недоступным (старый воркер без /queue отвечает 404)
    const [j, h, q] = await Promise.all([api.jobs(), api.status(), api.queueList().catch(() => null)])
    jobs.value = j || []
    procQueue.value = q || { paused: false, items: [] }
    // у трека пресет, которого нет в названиях (создан из MCP / в другой вкладке) — перечитать
    // (один раз на незнакомый id: удалённый пресет иначе перечитывался бы на каждом опросе)
    const unknown = jobs.value.flatMap((x) => (x.sound_presets || []).map((sp) => sp.id))
      .filter((id) => !presetNames.value[id] && !presetAsked.has(id))
    if (unknown.length) { unknown.forEach((id) => presetAsked.add(id)); loadPresetNames() }
    health.value = h
  } catch {
    health.value = null
  }
  compat.onHealth(health.value)
  // ход импорта рецептов — отдельно: старый воркер без /recipes не должен ронять обновление
  api.recipeImports().then((l) => { imports.value = activeImports(l) }).catch(() => { imports.value = [] })
  // сводка статус-бара отдельно: старый воркер без /stats не должна ронять обновление
  api.stats().then(s => { stats.value = s }).catch(() => { stats.value = null })
  try { playerState.value = await api.audioState() } catch {}
}
onRefresh(refresh)

// экспорт рецепта песни трека: файл правил без аудио (диалог сохранения)
async function exportRecipe(j) {
  try {
    const r = await api.exportRecipe(j.id)
    if (!r?.path) return
    // что в рецепт не вошло (версии до журнала, правки с чужими треками) — человек должен знать
    const lost = (r.warnings || []).length ? '\n\n' + t('recipe.export.warnings') + '\n— ' + r.warnings.join('\n— ') : ''
    alert(t('recipe.export.saved', { path: r.path }) + lost)
  } catch (e) {
    alert(String(e))
  }
}

// у упавшего импорта рецепта: продолжить с того же шага или убрать строку
async function importAction(imp, action) {
  try { await api.recipeImportAction(imp.id, action) } catch (e) { alert(String(e)) }
  refresh()
}

async function togglePlay(j) {
  await toggleArtifact(`m${j.id}`, `#${j.id} ${j.title}`, () => api.playAudio(j.id),
    { jobId: j.id, file: j.audio_file, dur: j.duration_sec })
}

async function cancel(id) {
  await api.cancel(id)
  refresh()
}
// метка «голос в инструментале»: где в треке без голоса звучит дорожка голоса
const leakTip = (j) => t('queue.vocalLeak.tip', { at: j.vocal_leak.split(',').map((s) => fmtDur(Number(s))).join(', ') })
// «повторить» упавшую или отменённую джобу: снова в очередь с теми же параметрами
const canRetry = (j) => j.status === 'error' || j.status === 'canceled'
// действующий движок генерации воркера ("torch" | "cpp-q8" …; '' — не известен): сид повторяет исполнение
// только на том же движке — перед повтором по сиду предупреждаем
const currentEngine = ref('')
async function loadEngine() {
  try { currentEngine.value = currentEngineLabel(await api.workerConfig()) } catch { currentEngine.value = '' }
}
onMounted(loadEngine)
// повтор по сиду на другом движке — с предупреждением; тот же движок — сразу. Движок перечитывается:
// его могли переключить не из этого окна (MCP config_set)
async function withEngineCheck(j, action) {
  await loadEngine()
  const warn = retryWarning(j, currentEngine.value)
  if (!warn) return action()
  askConfirm(t('engine.retryWarning.title'), warn, action)
}
async function doRetry(j) {
  try {
    await api.retryJob(j.id)
  } catch (e) {
    alert(String(e))
  }
  refresh()
}
const retry = (j) => withEngineCheck(j, () => doRetry(j))

function deleteJob(j) {
  askConfirm(t('queue.delete.title', { id: j.id, title: j.title }),
    t('queue.delete.text'),
    async () => {
      try { await api.deleteJob(j.id) } catch (e) { alert(String(e)) }
      refresh()
    })
}

const reuseJob = (j) => withEngineCheck(j, () => fillFromJob(j))
function fillFromJob(j) {
  title.value = j.title
  styleOverride.value = j.style
  lyrics.value = j.lyrics
  seed.value = j.seed || null
  cot.value = j.cot === 'melody' ? 'melody' : 'full'   // «без плана» в форме нет
  temperature.value = j.temperature || CHARACTER.temperature.def
  cfgScale.value = j.cfg || CHARACTER.cfg.def
  openNewTrack()
}

function openListen(j) {
  api.openURL(`${serverURL.value}/listen/${j.id}`)
}

async function saveMp3(j) {
  if (!j.mp3_file) {
    playBusy.value = { ...playBusy.value, ['mp3' + j.id]: true }
    try {
      const r = await api.ensureMp3(j.id)
      j.mp3_file = (r && r.file) || 'audio.mp3'
    } catch (e) {
      planErr.value = 'mp3: ' + e
      planOpen.value = true
      return
    } finally { playBusy.value = { ...playBusy.value, ['mp3' + j.id]: false } }
  }
  await saveAudio(j, j.mp3_file || 'audio.mp3')
}

async function saveAudio(j, file) {
  const f = file || j.audio_file
  if (f) await api.saveAudio(j.id, f)
}

// раздел в основном контексте: '' — список треков (главная). Прежние флаги —
// алиасы на section, вся логика открытия/зактия работает как раньше
const section = ref('')
const sectionFlag = (name) => computed({
  get: () => section.value === name,
  set: (v) => {
    if (v) section.value = name
    else if (section.value === name) section.value = ''
  },
})
const settingsPage = sectionFlag('settings')
const voicesPage = sectionFlag('voices')
const instrumentsPage = sectionFlag('instruments')   // «Инструменты»: дорожка через цепочку звукового движка
const recipeImport = ref(false)      // окно «Импорт рецепта»
// «Варианты: N» — сколько треков с разными seed; выбранное число помнится между запусками
const FAN_OPTIONS = [2, 3, 4, 5].map((n) => ({ value: n, label: String(n) }))
const fanN = ref(FAN_DEFAULT)
try { fanN.value = fanCount(localStorage.getItem('yue_fan_n')) } catch { /* хранилище недоступно — по умолчанию */ }
watch(fanN, (v) => { try { localStorage.setItem('yue_fan_n', String(fanCount(v))) } catch { /* не сохранили — не беда */ } })
const trackSearch = ref(null)        // поиск по трекам: Ctrl+F ставит в него фокус
const filtersOpen = ref(false)       // узкое окно: фильтры списка развёрнуты под поиском
const imports = ref([])             // импорты рецептов: ход над списком треков
const replayJob = ref(null)          // «переиграть в другом стиле»: трек или null
const studioJob = ref(null)
// главная — список треков; форма нового трека — отдельная страница
const newTrackPage = sectionFlag('new')

// «＋ Новый трек» — раздел формы (из студии закрывает её и открывает форму)
function openNewTrack() {
  navGo('new')
}

// Esc закрывает модалку нового трека — но не из-под открытых поверх плана/библиотеки
function onNewTrackKey(e) {
  if (e.key === 'Escape' && newTrackPage.value && !planOpen.value  && !myStylesOpen.value) newTrackPage.value = false
}
onMounted(() => window.addEventListener('keydown', onNewTrackKey))
onUnmounted(() => window.removeEventListener('keydown', onNewTrackKey))

// Ctrl/Cmd+F — к списку треков и в поиск (пробел и стрелки плеера — в PlayerBar.vue)
async function onSearchKey(e) {
  if (keyAction(e) !== 'search') return
  e.preventDefault()
  navGo('tracks')
  await nextTick()
  trackSearch.value?.focus()
}
onMounted(() => window.addEventListener('keydown', onSearchKey))
onUnmounted(() => window.removeEventListener('keydown', onSearchKey))

// страницы в меню «⋮»: треки/голоса переключаются, настройки просто открываются
const navOpen = ref(false)
// окно «Что это / Что нового»: при запуске — по welcome.js, из меню — всегда
const APP_VERSION = RELEASE_NOTES[0]?.version || ''
function readWelcome() {
  try { return JSON.parse(localStorage.getItem(WELCOME_KEY) || 'null') } catch { return null }
}
const welcome = ref(welcomeTab(readWelcome(), APP_VERSION))   // 'about' | 'news' | null
function closeWelcome(dontShow) {
  try {
    localStorage.setItem(WELCOME_KEY, JSON.stringify(welcomeClosed(readWelcome(), APP_VERSION, dontShow)))
  } catch { /* без localStorage окно просто покажется снова */ }
  welcome.value = null
}

// переход по разделам левого блока: «список треков» — главная; из студии выходим
function navGo(page) {
  navOpen.value = false
  if (studioJob.value) {
    studioJob.value = null
    refresh()
  }
  section.value = page === 'tracks' ? '' : page
}

// итог записи очереди («готово → #N») — открыть эту версию в студии
async function openFromQueue(id) {
  await refresh()
  const j = (jobs.value || []).find((x) => x.id === id)
  if (!j) return
  navOpen.value = false
  section.value = ''
  studioJob.value = j
}

// «Импорт трека» в списке треков: файл → воркер (транскрипция) → трек открывается в студии
const importBusy = ref(false)
async function importTrack() {
  importBusy.value = true
  try {
    const r = await api.importTrack()
    if (r && r.id) await onImported(r)
  } catch (e) { alert(String(e)) } finally { importBusy.value = false }
}

async function onImported(r) {
  await refresh()
  const j = (jobs.value || []).find((x) => x.id === r.id)
  if (j) studioJob.value = j
  if (r.transcribe_error) {
    planErr.value = t('track.import.err')
    planOpen.value = true
  }
}

// сохранённые голоса прямо в форме: выбор карточки = слот вокала + seed
const voiceCards = ref([])
const voicePick = ref('')

async function loadVoiceCards() {
  try { voiceCards.value = (await api.voices()) || [] } catch { voiceCards.value = [] }
}

const voiceOptions = computed(() => voiceCards.value.map((v) => ({ value: String(v.id), label: v.name })))

function onVoicePick() {
  const v = voiceCards.value.find((x) => String(x.id) === voicePick.value)
  if (!v) return
  let p
  try { p = normalizeVoiceParams(JSON.parse(v.params || '{}')) } catch { return } // битые params карточки — без голоса
  // дескриптор — в слот вокала, seed карточки — в форму (точная строка стиля очищается, иначе перекрыла бы слоты)
  slots.value.vocals = voiceDescriptor(p)
  styleOverride.value = ''
  seed.value = v.seed || null
}

const planOpen = ref(false)
const planBusy = ref(false)
const planErr = ref('')
const planInfo = ref(null)   // {seed, seconds, truncated} — как план получен
const planAbc = ref('')
const customAbc = ref('')    // прикреплённый план: скелет трека в форме
const abcOpen = ref(false)
const submitErr = ref('')

function setPlanAbc(abc, info) {
  planAbc.value = abc
  planInfo.value = info
  planErr.value = ''
  planOpen.value = true
}

async function makePlan() {
  if (!canSubmit.value) return
  planBusy.value = true
  planErr.value = ''
  planOpen.value = true
  const usedSeed = seed.value ? Number(seed.value) : Math.floor(Math.random() * 1e9)
  try {
    const r = await api.plan({
      style: finalStyle(),
      lyrics: cleanLyrics(lyrics.value),
      seed: usedSeed,
      cot: cot.value === 'off' ? 'full' : cot.value,
    })
    setPlanAbc(r.abc, { seed: usedSeed, seconds: r.seconds, truncated: r.truncated })
  } catch (e) {
    planErr.value = String(e)
  } finally { planBusy.value = false }
}

// план из диалога → прикрепить к форме: скелет из ABC, а жанр/инструменты/ритм,
// дугу и сид задаём настройками; запуск — главной кнопкой генерации
function usePlanAbc(abc) {
  customAbc.value = (abc || '').trim()
  submitErr.value = ''
  planOpen.value = false
  openNewTrack()   // план мог открыться не из формы (свои треки) — показать форму с ним
}

// «ноты» — итоговый план: с прикреплённым показываем его, без — генерируем новый
function notesOpen() {
  if (customAbc.value.trim()) {
    setPlanAbc(customAbc.value, {})
    return
  }
  makePlan()
}

// метки приёмов переезжают в новую версию трека: правки плана накопительные,
// ролл ребёнка показывает, что уже ломали (формат — состояние студии, см. StudioPage)
function inheritTrickMarks(newJobId) {
  const marks = planInfo.value && planInfo.value.marks
  if (!marks || !marks.length || !newJobId) return
  try {
    const key = 'yue_studio_state'
    const st = JSON.parse(localStorage.getItem(key) || '{}')
    st.jobs = st.jobs || {}
    st.jobs[newJobId] = { ...(st.jobs[newJobId] || {}), trickMarks: marks }
    localStorage.setItem(key, JSON.stringify(st))
  } catch {}
}

async function loadJobAbc(j) {
  if (!j.abc_file) return
  planErr.value = ''
  try {
    const abc = await api.jobAbcText(j.id, j.abc_file)
    setPlanAbc(abc, { seed: j.seed, seconds: null, truncated: false, fromJob: j.id })
  } catch (e) {
    planErr.value = String(e)
    planOpen.value = true
  }
}

// Remix: трек → ABC (SheetSage2 на воркере)
async function transcribeFromTrack() {
  planErr.value = ''
  planBusy.value = true
  planOpen.value = true
  try {
    const r = await api.transcribeFile()
    if (!r) { planOpen.value = false; return }   // диалог отменён
    setPlanAbc(r.abc, { seed: null, seconds: r.seconds, truncated: false, fromTrack: true })
  } catch (e) {
    planErr.value = String(e)
  } finally { planBusy.value = false }
}

// кнопки тегов секций: тег — отдельной строкой в место курсора, поле остаётся в фокусе
const lyricsEl = ref(null)
async function addSectionTag(tag) {
  const el = lyricsEl.value
  const at = el ? [el.selectionStart, el.selectionEnd] : [lyrics.value.length, lyrics.value.length]
  const r = insertSectionTag(lyrics.value, at[0], at[1], tag)
  lyrics.value = r.text
  await nextTick()
  if (!el) return
  el.focus()
  el.setSelectionRange(r.caret, r.caret)
}

const metricsModal = ref(null)
function openMetrics(j, preset) {
  metricsModal.value.openFor(j, preset)
}

onMounted(loadPresetNames)
onMounted(async () => {
  onVolume()
  serverURL.value = await api.getServerURL()
  refresh()
  loadVoiceCards()
  timer = setInterval(refresh, 3000)
  window.addEventListener('click', onWindowClick)
})
onUnmounted(() => { clearInterval(timer); window.removeEventListener('click', onWindowClick) })

function progressTip(j) {
  const parts = [t('queue.progress.' + (j.stage || 'plan'))]
  if (j.tokens) parts.push(j.tokens + ' tok')
  if (j.tok_per_s) parts.push(j.tok_per_s + ' ' + t('queue.tokPerSec'))
  if (j.elapsed_sec) parts.push(fmtDur(j.elapsed_sec))
  return parts.join(' · ')
}

function onWindowClick(e) {
  if (!e.target.closest('.slot-box')) openSlot.value = ''
  if (!e.target.closest('.nav-wrap')) navOpen.value = false
}
</script>

<template>
  <TitleBar />
  <div v-if="compat.warning.value" class="compat-banner">
    <AppIcon name="alert" /> {{ compat.warning.value }}
    <button class="ghost icon" :title="t('common.close')" @click="compat.dismiss"><AppIcon name="x" /></button>
  </div>
  <div class="app-shell">
  <SideBar :logo="logoUrl" :stats="stats" :health="{ up: !!health, title: healthTitle }"
           :section="section" :studio-job="studioJob" :queue="procQueue" :worker-url="serverURL" :theme="theme" :locale="locale"
           @nav="navGo" @open-job="openFromQueue" @about="welcome = 'about'"
           @theme="toggleTheme" @locale="setLocale(locale === 'ru' ? 'en' : 'ru')" />
  <div class="app-col">

  <!-- всё под шапкой прокручивается само: шапка с плеером всегда на виду -->
  <div class="app-body">
  <!-- разделы — в основном контексте (не модалками): закрываются «вернуться» в шапке и ✕;
       библиотека своих треков и корпуса — по-прежнему модалки поверх формы -->
  <div v-if="!studioJob && section" class="ctx-view">
    <!-- форма нового трека — страница раздела «Новый трек»: слева блоки 1–3 (что за трек),
         справа «Отправка» (что уйдёт модели и запуск), прилипает при прокрутке -->
    <PageView v-if="section === 'new'" :title="t('nav.new')">
      <div class="page-cols">
      <div class="page-main">
        <PageBox :title="'1 · ' + t('nt.step.style')" :hint="t('nt.step.style.sub')">
            <div class="lib-row" :title="t('form.lib.tip')">
              <VSelect v-model="libGroup" :options="libGroupOptions" :placeholder="t('form.lib.group')" searchable @update:model-value="onLibGroupChange()" />
              <VSelect v-model="libStyle" :options="libStyleOptions" :disabled="!libGroup" :placeholder="t('form.lib.style')" @update:model-value="onLibStyleChange()" />
              <button v-if="currentItem" class="ghost small-btn" :title="t('form.lib.exact.tip')" @click="onLibExact">{{ t('form.lib.exact') }}</button>
              <button class="ghost small-btn" :title="t('my.save.tip')" @click="openStyleSave">{{ t('my.save') }}</button>
              <button class="ghost small-btn" :title="t('my.list.tip')" @click="myStylesOpen = true">{{ t('my.list') }}</button>
            </div>
            <input v-model="title" :placeholder="t('form.name')" />
        </PageBox>

        <PageBox :title="'2 · ' + t('nt.step.sound')" :hint="t('nt.step.sound.sub')">
            <div class="slots">
              <div v-for="key in slotKeys" :key="key" class="slot-box" :title="t('slot.' + key + '.tip')">
                <span>{{ t('slot.' + key + '.label') }}</span>
                <input v-model="slots[key]" :placeholder="t('slot.' + key + '.hint')"
                       @focus="openSlot = key" @click.stop @input="openSlot = key" />
                <ul v-if="openSlot === key && slotFiltered(key).length" class="slot-drop">
                  <li v-for="o in slotFiltered(key)" :key="o[0]"
                      @mousedown.prevent="slots[key] = locale === 'en' ? o[1] : o[0]; openSlot = ''"
                  >{{ locale === 'en' ? o[1] : o[0] }}</li>
                </ul>
              </div>
              <label>
                <span>BPM</span>
                <input v-model.number="slots.bpm" type="number" min="40" max="250" placeholder="—" />
              </label>
            </div>
            <div class="lib-row" :title="t('form.voice.tip')">
              <VSelect v-if="voiceCards.length" v-model="voicePick" :options="voiceOptions"
                       :placeholder="t('form.voice.ph')" @update:model-value="onVoicePick()" />
              <!-- своих голосов нет — подсказать, где их подобрать -->
              <button v-else class="ghost" @click="navGo('voices')">{{ t('form.voice.pick') }}</button>
            </div>
            <!-- инструментальная стойка: группа → инструмент → эффект → «Добавить», добавленное — фишками -->
            <RackPicker v-model="rackSel" />
            <details>
              <summary>
                {{ t('form.styleline') }}
                <span v-if="styleOverride" class="muted">{{ t('form.styleline.manual') }}</span>
                <span v-else class="muted">{{ t('form.styleline.auto') }}</span>
              </summary>
              <textarea v-model="styleLine" rows="3" :placeholder="t('form.styleline.ph')"></textarea>
              <button v-if="styleOverride" class="ghost small-btn" style="margin-top:6px"
                      :title="t('form.styleline.follow.tip')"
                      @click="styleOverride = ''">{{ t('form.styleline.follow') }}</button>
            </details>

        </PageBox>

        <PageBox :title="'3 · ' + t('nt.step.lyrics')" :hint="t('form.lyrics')">
          <template #tools>
              <button class="toggle small-btn" :class="{ on: noLyrics }" :title="t('form.nowords.tip')" @click="noLyrics = !noLyrics">{{ t('form.nowords') }}</button>
              <select v-if="noLyrics" v-model="durMode" class="dur-select" :title="t('form.dur.tip')">
                <option v-for="o in durOptions" :key="o.id" :value="o.id">{{ t('dur.' + o.id) }}</option>
              </select>
          </template>
            <!-- теги секций: вставка в место курсора; без тегов YuE2 расставит части сама -->
            <div v-if="!noLyrics" class="lyrics-tags">
              <button v-for="s in SECTION_TAGS" :key="s.id" class="ghost small-btn" :title="s.tag"
                      @click="addSectionTag(s.tag)">{{ t('lyrics.tag.' + s.id) }}</button>
              <span class="lyrics-tags-hint muted">{{ t('lyrics.tags.hint') }}</span>
            </div>
            <textarea ref="lyricsEl" v-model="lyrics" rows="10" :disabled="noLyrics" :placeholder="noLyrics ? t('form.nowords.ph') : ''"></textarea>
            <!-- «Импорт ABC»: готовый план как скелет трека; жанр, инструменты, ритм — из формы -->
            <div class="abc-row" :title="t('form.abc.tip')">
              <button class="toggle small-btn" :class="{ on: abcOpen || !!customAbc.trim() }"
                      @click="abcOpen = !abcOpen">{{ t('form.abc.btn') }}</button>
              <span v-if="customAbc.trim() && !abcOpen" class="muted">
                {{ t('form.abc.on') }} · {{ customAbc.trim().split('\n').length }} {{ t('form.abc.lines') }}
              </span>
              <button v-if="customAbc.trim()" class="ghost small-btn" @click="customAbc = ''">{{ t('form.abc.clear') }}</button>
            </div>
            <textarea v-if="abcOpen" v-model="customAbc" rows="8" class="abc" spellcheck="false"
                      :placeholder="t('form.abc.ph')"></textarea>
        </PageBox>
      </div>

      <aside class="page-side">
        <PageBox :title="t('nt.send')" :hint="t('nt.send.sub')">
            <!-- итоговая строка стиля, которая уйдёт модели; кириллица — предупреждение, не перевод -->
            <div class="row style-out">
              <span class="style-out-cap">{{ t('form.styleout') }}:</span>
              <span class="compiled" :title="compiledStyle">{{ compiledStyle || t('form.style.empty') }}</span>
            </div>
            <div v-if="hasCyrillic(sentStyle)" class="style-cyr">{{ t('form.style.cyr') }}</div>
            <PresetChips v-model="soundPresetIds" />
            <div class="nt-actions">
              <button class="primary nt-submit" :disabled="submitting || !canSubmit" @click="submit">
                {{ submitting ? t('form.submitting') : t('form.submit') }}
              </button>
              <!-- варианты: N треков с разными seed — число выбирается, запоминается -->
              <div class="nt-fan">
                <VSelect v-model="fanN" :title="t('form.fan.n.tip')" :options="FAN_OPTIONS" />
                <button class="primary alt" :disabled="submitting || !canSubmit" @click="submitFan(fanN)"
                        :title="t('form.fan.tip', { n: fanN })">{{ t('form.fan', { n: fanN }) }}</button>
              </div>
              <button class="ghost" :disabled="planBusy || !canSubmit" @click="notesOpen"
                      :title="t('form.notes.tip')">
                {{ planBusy ? t('form.planning') : t('form.notes') }}
              </button>
              <button class="ghost" :disabled="submitting || !canSubmit" @click="submitDraft"
                      :title="t('form.draft.tip')">{{ t('form.draft') }}</button>
              <span v-if="submitErr" class="error submit-err" :title="submitErr">{{ submitErr }}</span>
            </div>
            <details class="form-advanced">
              <summary>{{ t('form.advanced') }}<span v-if="advancedSummary" class="muted"> · {{ advancedSummary }}</span></summary>
              <div class="row">
                <label class="seed" :title="t('form.seed.tip')">seed <input v-model.number="seed" type="number" :placeholder="t('form.seed.ph')" /></label>
                <!-- «без плана» в форме нет: он отключает студию (ноты, приёмы, драматургию); в API/MCP остался -->
                <div class="cot-radios" :title="t('form.cot.tip')">
                  <span class="arc-title">{{ t('form.cot.title') }}:</span>
                  <button v-for="c in ['full', 'melody']" :key="c" class="toggle small-btn"
                          :class="{ on: cot === c }" @click="cot = c">{{ t('form.cot.' + c) }}</button>
                </div>
              </div>

              <div class="row arc-row">
                <span class="arc-title">{{ t('arc.title') }}:</span>
                <button v-for="a in ['', 'build', 'wave', 'burst']" :key="a" class="toggle small-btn"
                        :class="{ on: arcKind === a }" :title="t('arc.' + (a || 'flat') + '.tip')"
                        @click="arcKind = a">{{ t('arc.' + (a || 'flat')) }}</button>
              </div>

              <div class="row character-row">
                <span class="arc-title">{{ t('character.title') }}:</span>
                <label class="character-knob" :title="t('character.temperature.tip')">
                  {{ t('character.temperature') }}
                  <input type="range" v-model.number="temperature" :min="CHARACTER.temperature.min"
                         :max="CHARACTER.temperature.max" :step="CHARACTER.temperature.step" />
                  <span class="character-val">{{ temperature }}</span>
                </label>
                <label class="character-knob" :title="t('character.cfg.tip')">
                  {{ t('character.cfg') }}
                  <input type="range" v-model.number="cfgScale" :min="CHARACTER.cfg.min"
                         :max="CHARACTER.cfg.max" :step="CHARACTER.cfg.step" />
                  <span class="character-val">{{ cfgScale }}</span>
                </label>
                <button class="ghost small-btn" :disabled="characterDefault" @click="characterReset">{{ t('character.reset') }}</button>
              </div>
            </details>
        </PageBox>
      </aside>
      </div>
    </PageView>

    <SettingsPage v-if="section === 'settings'" v-model:server-url="serverURL"
                  @close="settingsPage = false" @saved="refresh(); loadEngine()" />
    <VoicesPage v-if="section === 'voices'"
                @close="voicesPage = false; loadVoiceCards()" />
    <InstrumentsPage v-if="section === 'instruments'" @close="instrumentsPage = false; refresh()" />
    <PresetsPage v-if="section === 'presets'" />
  </div>
  <StudioPage v-else-if="studioJob" :job="studioJob"
              @close="studioJob = null; refresh()"
              @open-metrics="openMetrics" />

  <main v-else>
    <!-- страница треков: заголовок, справа — импорт (свой трек / рецепт) и «Новый трек» -->
    <PageView :title="t('nav.tracks')" class="track-list">
      <template #actions>
        <details class="menu-pop down">
          <summary class="ghost-btn" :class="{ busy: importBusy }">
            <AppIcon name="folder" /> {{ importBusy ? t('track.import.busy') : t('import.menu') }}</summary>
          <ul class="nav-menu" @click="closeMenu">
            <li :title="t('track.import.tip')" @click="!importBusy && importTrack()">{{ t('track.import.btn') }}</li>
            <li :title="t('recipe.importMenu.tip')" @click="recipeImport = true">{{ t('recipe.importMenu.btn') }}</li>
          </ul>
        </details>
        <button class="primary" @click="openNewTrack"><AppIcon name="plus" /> {{ t('nav.new') }}</button>
      </template>
      <div v-for="imp in imports" :key="imp.id" class="recipe-progress" :class="{ error: imp.status === 'error' }">
        {{ imp.status === 'error' ? t('recipe.progress.error', { title: imp.title, error: imp.error })
                                  : t('recipe.progress', { title: imp.title, progress: imp.progress || '…' }) }}
        <template v-if="imp.status === 'error'">
          <button class="ghost small-btn" @click="importAction(imp, 'retry')">{{ t('recipe.retry') }}</button>
          <button class="ghost small-btn" @click="importAction(imp, 'dismiss')">{{ t('recipe.dismiss') }}</button>
        </template>
      </div>
      <p v-if="!jobs.length" class="muted">{{ t('common.empty') }}</p>
      <template v-else>
        <div class="queue-tools">
          <!-- поиск первым и шире, фильтры — следом; импорт — в строке заголовка страницы -->
          <input ref="trackSearch" v-model="qf.q" class="queue-search" :placeholder="t('queue.filter.search')" />
          <!-- в узком окне фильтры прячутся за кнопкой со счётчиком включённых (стили — .queue-filters) -->
          <button class="ghost queue-filter-btn" :class="{ on: filtersOpen }" @click="filtersOpen = !filtersOpen">
            {{ t('queue.filters') }}<template v-if="activeFilterCount(qf)"> · {{ activeFilterCount(qf) }}</template></button>
          <div class="queue-filters" :class="{ open: filtersOpen }">
            <VSelect v-model="qf.status" :options="qStatusOptions" />
            <VSelect v-model="qf.period" :options="qPeriodOptions" />
            <VSelect v-model="qf.dur" :options="qDurOptions" />
            <VSelect v-model="qf.draft" :options="qDraftOptions" />
            <VSelect v-model="qf.folder" :options="qFolderOptions" :title="t('queue.folder.tip')" />
          </div>
        </div>
        <p class="muted">{{ filteredJobs.length
          ? t('queue.filter.shown', { shown: queuePage.length, total: filteredJobs.length })
          : t('queue.filter.none') }}
          <button v-if="openOnPage >= 2" class="ghost small-btn collapse-all" @click="collapseAllJobs">{{ t('queue.collapseAll') }}</button></p>
        <!-- таблица треков: шапка столбцов и строки на одной сетке (ширины .col-*); неоднозначное
             (версии, статус) — в подсказках -->
        <div v-if="queuePage.length" class="job-table" :class="{ 'has-open': openOnPage }">
        <!-- шапка столбцов: те же ширины, что у строк (.col-*) -->
        <div class="job-row job-head-row">
          <span class="tl-num">№</span>
          <span class="tl-play-cell"></span>
          <span class="job-main">{{ t('queue.col.title') }}</span>
          <span class="col-kids">{{ t('queue.col.versions') }}</span>
          <span class="col-folder">{{ t('queue.col.folder') }}</span>
          <span class="col-dur">{{ t('queue.col.dur') }}</span>
          <span class="col-when">{{ t('queue.col.date') }}</span>
          <span class="col-actions"></span>
        </div>
        <article v-for="j in queuePage" :key="j.id" class="job"
                 :class="[j.status, { playing: songPlaying(j), open: openJobs.has(j.id) }]">
        <div class="job-row" :class="{ open: openJobs.has(j.id) }" @click="toggleOpenJob(j.id)">
          <!-- играющий сейчас трек: значок вместо номера, строка отмечена (стили .job.playing) -->
          <span class="tl-num" :title="'#' + j.id"><AppIcon v-if="songPlaying(j)" name="volume" /><template v-else>{{ j.id }}</template></span>
          <span class="tl-play-cell">
            <button v-if="j.status === 'done' && j.audio_file" class="tl-play"
                    :class="{ 'is-playing': isPlaying('m' + headOf(j).id) }" :disabled="playBusy['m' + headOf(j).id]"
                    :title="t('queue.play')" @click.stop="togglePlay(headOf(j))">
              <AppIcon :name="isPlaying('m' + headOf(j).id) ? 'pause' : 'play'" /></button>
            <span v-else class="status-mini" :class="j.status" :title="statusLabelC[j.status] || j.status"></span>
          </span>
          <input v-if="titleEdit && titleEdit.id === j.id" v-model="titleEdit.value" class="title-edit job-name"
                 @keydown.enter="saveRename(j)" @keydown.esc="titleEdit = null" @blur="saveRename(j)" @click.stop />
          <span v-else class="job-main">
           <span class="job-title-line">
            <span class="job-name" :title="j.title">{{ j.title }}</span>
            <span v-if="j.draft" class="badge draft">{{ t('queue.draft') }}</span>
            <span v-else-if="j.status === 'error'" class="status" :class="j.status">{{ statusLabelC[j.status] || j.status }}</span>
            <span v-else-if="j.status === 'queued'" class="status" :class="j.status">{{ statusLabelC[j.status] || j.status }}</span>
            <span v-else-if="j.status === 'done' && headOf(j).vocal_leak" class="badge warn" :title="leakTip(headOf(j))"><AppIcon name="alert" /></span>
            <span v-if="j.status === 'done' && headOf(j).truncated" class="badge warn" :title="t('queue.truncated.tip')">{{ t('queue.truncated') }}</span>
            <!-- после цепочки v-if/v-else-if пометок статуса: внутри неё ломала бы «ошибку», «в очереди», ⚠ -->
            <span v-if="songDraft(j)" class="badge wip" :title="t('queue.wip.tip')">{{ t('queue.wip') }}</span>
            <span v-if="j.status === 'running'" class="job-progress" :title="progressTip(j)">
              <span class="progress-track slim" :class="{ indet: j.progress_pct == null }">
                <span v-if="j.progress_pct != null" class="progress-fill" :style="{ width: j.progress_pct + '%' }"></span>
              </span>
              <span class="muted progress-label">
                <template v-if="j.progress_pct != null">{{ j.progress_pct }}%</template>
                <template v-else>{{ t('queue.progress.' + (j.stage || 'plan')) }}</template>
                <template v-if="j.tok_per_s"> · {{ j.tok_per_s }} {{ t('queue.progress.tps') }}</template>
              </span>
            </span>
            <button class="ghost icon job-rename" :title="t('queue.rename.tip')" @click.stop="startRename(j)"><AppIcon name="pencil" /></button>
           </span>
           <!-- коротко, что за трек: жанр, голос, гитары — без раскрытия -->
           <span v-if="j.style" class="job-style-line" :title="j.style">{{ styleSummary(j.style) }}</span>
          </span>
          <span class="col-kids"><span v-if="kidCount(j)" class="tl-pill kids-badge" :title="t('queue.kids.tip')"
                @click.stop="toggleOpenJob(j.id)">{{ kidCount(j) }}</span></span>
          <span class="col-folder"><span v-if="(j.folder || '').trim()" class="tl-pill job-folder" :title="j.folder.trim()">{{ j.folder.trim() }}</span></span>
          <span class="tl-meta col-dur">{{ j.duration_sec ? fmtDur(j.duration_sec) : '' }}</span>
          <span class="tl-meta col-when" :title="j.created_at">{{ fmtWhen(j.created_at) }}</span>
          <!-- действия видны всегда: «Студия →» и «⋯» (скачать, ноты, переиграть, рецепт, удалить) -->
          <span class="col-actions" @click.stop>
            <button v-if="j.status === 'done' && j.audio_file" class="ghost small-btn" @click="studioJob = headOf(j)">{{ t('queue.studio') }}</button>
            <details v-if="j.status === 'done' && j.audio_file" class="menu-pop down">
              <summary class="ghost-btn" :title="t('queue.more.tip')"><AppIcon name="more-h" /></summary>
              <ul class="nav-menu" @click="closeMenu">
                <li :title="t('queue.save.flac.tip')" @click="saveAudio(headOf(j))"><AppIcon class="nav-ico" name="save" />{{ t('queue.dl.flac') }}</li>
                <li :title="t('queue.save.mp3.tip')" @click="!playBusy['mp3' + headOf(j).id] && saveMp3(headOf(j))"><AppIcon class="nav-ico" name="save" />{{ t('queue.dl.mp3') }}</li>
                <li v-if="headOf(j).abc_file" :title="t('queue.notes.save.tip')" @click="saveAudio(headOf(j), headOf(j).abc_file)"><AppIcon class="nav-ico" name="save" />{{ t('queue.download.abc') }}</li>
                <li :title="t('queue.browser.tip')" @click="openListen(headOf(j))"><AppIcon class="nav-ico" name="globe" />{{ t('queue.browser') }}</li>
                <li v-if="headOf(j).abc_file" :title="t('queue.notes.tip')" @click="loadJobAbc(headOf(j))"><AppIcon class="nav-ico" name="music" />{{ t('queue.notes') }}</li>
                <li :title="t('queue.repeat.tip')" @click="reuseJob(j)"><AppIcon class="nav-ico" name="repeat" />{{ t('queue.repeat') }}</li>
                <li :title="t('replay.menu.tip')" @click="replayJob = headOf(j)"><AppIcon class="nav-ico" name="music" />{{ t('replay.menu') }}</li>
                <li v-if="!j.draft" :title="t('recipe.export.tip')" @click="exportRecipe(j)"><AppIcon class="nav-ico" name="save" />{{ t('recipe.export') }}</li>
                <li class="danger" :title="t('queue.delete.tip')" @click="deleteJob(j)"><AppIcon class="nav-ico" name="x" />{{ t('queue.delete') }}</li>
              </ul>
            </details>
            <button class="ghost icon job-caret" :title="t('queue.details.tip')" @click="toggleOpenJob(j.id)"><AppIcon :name="openJobs.has(j.id) ? 'chev-up' : 'chev-down'" /></button>
          </span>
        </div>
        <div v-if="openJobs.has(j.id)" class="job-detail">
            <!-- версии — первыми, подстроками под названием: оригинал и версии по порядку появления,
                 миксы-вклейки, материал; «Студия →» и ✕ у версии — при наведении -->
            <div v-if="kidCount(j) || (kidMixes[j.id] || []).length || kidMaterial(j.id).length" class="job-kids">
                <div v-for="v in [j, ...kidResults(j.id)]" :key="v.id" class="job-kid" :class="{ current: headOf(j).id === v.id, playing: isPlaying('m' + v.id) }">
                  <span class="tl-play-cell">
                    <button v-if="v.status === 'done' && v.audio_file" class="tl-play"
                            :class="{ 'is-playing': isPlaying('m' + v.id) }" :disabled="playBusy['m' + v.id]"
                            :title="t('queue.play')" @click="togglePlay(v)">
                      <AppIcon :name="isPlaying('m' + v.id) ? 'stop' : 'play'" /></button>
                    <span v-else class="status-mini" :class="v.status" :title="statusLabelC[v.status] || v.status"></span>
                  </span>
                  <button v-if="v.status === 'done'" class="kid-star" :class="{ head: headOf(j).id === v.id }"
                          :title="headOf(j).id === v.id ? t('queue.head.main') : t('queue.head.make.tip')"
                          :disabled="headOf(j).id === v.id" @click="makeHead(j, v)">
                    <AppIcon :name="headOf(j).id === v.id ? 'star-fill' : 'star'" /></button>
                  <span class="muted tl-num" style="width:auto">{{ v.id }}</span>
                  <span class="kid-title" :title="v.title">{{ v.id === j.id ? t('queue.kids.original') : v.title }}</span>
                  <span v-if="v.id !== j.id" class="badge">{{ t('queue.role.' + v.role) }}</span>
                  <span v-if="v.has_draft" class="badge wip" :title="t('queue.wip.tip')">{{ t('queue.wip') }}</span>
                  <span v-if="v.vocal_leak" class="badge warn" :title="leakTip(v)"><AppIcon name="alert" /></span>
                  <span v-if="v.truncated" class="badge warn" :title="t('queue.truncated.tip')">{{ t('queue.truncated') }}</span>
                  <span v-if="v.id !== j.id && v.status !== 'done'" class="status" :class="v.status">{{ statusLabelC[v.status] || v.status }}</span>
                  <span class="spacer"></span>
                  <span v-if="v.duration_sec" class="tl-meta">{{ fmtDur(v.duration_sec) }}</span>
                  <span class="kid-actions">
                    <button v-if="v.status === 'done'" class="ghost small-btn" @click="studioJob = v">{{ t('queue.studio') }}</button>
                    <button v-if="v.id !== j.id && canRetry(v)" class="ghost small-btn" :title="v.error || t('queue.retry.tip')" @click="retry(v)">{{ t('queue.retry') }}</button>
                    <button v-if="v.id !== j.id && v.status !== 'running'" class="ghost icon del" :title="t('queue.delete.tip')" @click="deleteJob(v)"><AppIcon name="x" /></button>
                  </span>
                </div>
                <div v-for="(v, n) in kidMixes[j.id] || []" :key="'mx' + v.file" class="job-kid" :class="{ playing: isPlaying(mixKey(j.id, v.file)) }">
                  <span class="tl-play-cell">
                    <button class="tl-play" :class="{ 'is-playing': isPlaying(mixKey(j.id, v.file)) }"
                            :disabled="playBusy[mixKey(j.id, v.file)]" :title="mixName(j.id, v)"
                            @click="playMix(j, v)">
                      <AppIcon :name="isPlaying(mixKey(j.id, v.file)) ? 'stop' : 'play'" /></button>
                  </span>
                  <span class="kid-star head" :title="t('queue.kids.mix')"><AppIcon name="disc" /></span>
                  <span class="kid-title" :title="mixName(j.id, v)">{{ mixName(j.id, v) }}</span>
                  <span v-if="n === 0" class="badge">{{ t('queue.kids.latest') }}</span>
                  <span class="spacer"></span>
                  <span v-if="fmtWhen(v.created_at)" class="tl-meta" :title="v.created_at">{{ fmtWhen(v.created_at) }}</span>
                  <span class="kid-actions"></span>
                </div>
                <div v-for="k in kidMaterial(j.id)" :key="k.id" class="job-kid" :class="{ playing: isPlaying('m' + k.id) }">
                  <span class="tl-play-cell">
                    <button v-if="k.status === 'done' && k.audio_file" class="tl-play"
                            :class="{ 'is-playing': isPlaying('m' + k.id) }" :disabled="playBusy['m' + k.id]"
                            :title="t('queue.play')" @click="togglePlay(k)">
                      <AppIcon :name="isPlaying('m' + k.id) ? 'stop' : 'play'" /></button>
                    <span v-else class="status-mini" :class="k.status" :title="statusLabelC[k.status] || k.status"></span>
                  </span>
                  <button v-if="k.status === 'done'" class="kid-star" :class="{ head: headOf(j).id === k.id }"
                          :title="headOf(j).id === k.id ? t('queue.head.main') : t('queue.head.make.tip')"
                          :disabled="headOf(j).id === k.id" @click="makeHead(j, k)">
                    <AppIcon :name="headOf(j).id === k.id ? 'star-fill' : 'star'" /></button>
                  <span class="muted tl-num" style="width:auto">{{ k.id }}</span>
                  <span class="kid-title" :title="k.title">{{ k.title }}</span>
                  <span class="badge">{{ t('queue.role.' + (k.role || (k.overdub_of ? 'overdub' : 'other'))) }}</span>
                  <span v-if="k.status !== 'done'" class="status" :class="k.status">{{ statusLabelC[k.status] || k.status }}</span>
                  <span class="spacer"></span>
                  <span v-if="k.duration_sec" class="tl-meta">{{ fmtDur(k.duration_sec) }}</span>
                  <span class="kid-actions">
                    <button v-if="k.status === 'done'" class="ghost small-btn" @click="studioJob = k">{{ t('queue.studio') }}</button>
                    <button v-if="canRetry(k)" class="ghost small-btn" :title="k.error || t('queue.retry.tip')" @click="retry(k)">{{ t('queue.retry') }}</button>
                    <button v-if="k.status !== 'running'" class="ghost icon del" :title="t('queue.delete.tip')" @click="deleteJob(k)"><AppIcon name="x" /></button>
                  </span>
                </div>
              </div>
          <!-- мета трека: сид, характер, свой ABC, head — одной приглушённой строкой;
               название и переименование живут в строке списка, здесь их нет -->
          <div class="job-meta-line">
            <span class="job-meta-extra">
              <span v-if="j.seed" class="muted">seed {{ j.seed }}</span>
              <span v-if="engineLabel(j)" class="badge exp" :title="t('engine.badge.tip')">{{ engineLabel(j) }}</span>
              <span v-if="j.cot && j.cot !== 'full'" class="muted" :title="t('queue.cot.tip')">{{ t('queue.cot.' + j.cot) }}</span>
              <span v-if="characterLabel(j)" class="muted" :title="t('character.title')">{{ characterLabel(j) }}</span>
              <span v-if="j.req_abc" class="badge" :title="t('form.abc.tip')">{{ t('queue.ownAbc') }}</span>
              <span v-if="headOf(j) !== j" class="badge current" :title="headOf(j).title"><AppIcon name="star-fill" /> {{ t('queue.head.badge', { id: headOf(j).id }) }}</span>
              <span v-if="j.status === 'error'" class="status error">{{ statusLabelC.error }}</span>
            </span>
            <span class="spacer"></span>
            <!-- папка: обычно просто иконка, клик — сменить; селект появляется только тогда -->
            <input v-if="folderNew && folderNew.id === j.id" v-model="folderNew.value" class="title-edit"
                   :placeholder="t('queue.folder.new.ph')" @keydown.enter="saveNewFolder(j)"
                   @keydown.esc="folderNew = null" @blur="saveNewFolder(j)" />
            <VSelect v-else-if="folderPick === j.id" :model-value="(j.folder || '').trim() || FOLDER_NONE" :options="jobFolderOptions" style="max-width: 200px"
                     :title="t('queue.folder.move.tip')" @update:model-value="(v) => { pickFolder(j, v); folderPick = null }" />
            <button v-else class="ghost icon" :title="t('queue.folder.move.tip')" @click="folderPick = j.id"><AppIcon name="folder" /></button>
            <button v-if="j.status === 'queued' || j.status === 'running'" class="ghost small-btn"
                    :title="t('queue.cancel.tip')" @click="cancel(j.id)">{{ t('queue.cancel') }}</button>
            <button v-if="!(j.status === 'done' && j.audio_file)" class="ghost icon del" :title="t('queue.delete.tip')" @click="deleteJob(j)"><AppIcon name="x" /></button>
            <button v-if="!(j.status === 'done' && j.audio_file)" class="ghost icon" :title="t('queue.repeat.tip')" @click="reuseJob(j)"><AppIcon name="repeat" /></button>
          </div>
          <div class="job-body">
            <div v-if="j.style" class="job-style">
              <div class="style-tags" :title="j.style">
                <template v-for="[slot, parts] in styleRows(j.style)" :key="slot">
                  <span class="style-tag-key">{{ styleTagLabel(slot) }}</span>
                  <span class="style-tag-val">{{ parts.join(', ') }}</span>
                </template>
              </div>
            </div>
            <JobPresets :job="j" :names="presetNames" @changed="refresh" />
          <p v-if="j.error" class="error">{{ j.error }}</p>
          <button v-if="canRetry(j)" class="ghost small-btn" :title="t('queue.retry.tip')" @click="retry(j)">{{ t('queue.retry') }}</button>
          </div>
        </div>
      </article>
        </div><!-- /job-table -->
        <!-- метка конца списка: попала в окно прокрутки — догружаются следующие треки -->
        <div v-if="queuePage.length < filteredJobs.length" ref="listEnd" class="list-end muted">{{ t('queue.more') }}</div>
      </template>
    </PageView>
  </main>
  </div>

  <!-- плеер — единственный внизу, под содержимым: транспорт стабилен, что бы ни было открыто -->
  <footer class="playerbar-footer">
    <PlayerBar :jobs="jobs" @play-job="togglePlay" @refresh="refresh" />
  </footer>
  </div><!-- /.app-col -->
  </div><!-- /.app-shell -->

  <!-- страницы-модалки поверх основного контента: ✕/Esc/клик по фону закрывают -->
  <!-- библиотека — после формы нового трека: открывается поверх неё, закрылась — форма на месте -->

  <RecipeImportModal v-if="recipeImport" @close="recipeImport = false" @started="refresh()" />
  <ReplayModal v-if="replayJob" :job="replayJob" @close="replayJob = null" @submitted="refresh()" />


  <PlanModal v-model:abc="planAbc" :open="planOpen" :busy="planBusy" :err="planErr"
             :info="planInfo"
             @close="planOpen = false" @use="usePlanAbc" @new-plan="makePlan" @from-track="transcribeFromTrack" />
  <MetricsModal ref="metricsModal" :jobs="jobs" />
  <WelcomeModal v-if="welcome" :tab="welcome" @close="closeWelcome" />
  <MyStylesModal v-if="myStylesOpen" @close="myStylesOpen = false"
                 @apply="(item) => { applyPreset(item, 'slots'); myStylesOpen = false }" />
  <ConfirmModal />

  <!-- «сохранить стиль»: спросить имя и положить форму в свои стили -->
  <div v-if="styleSaveOpen" class="modal-backdrop">
    <div class="modal" style="width: min(480px, 92vw)">
      <div class="modal-head">
        <h2>{{ t('my.save.title') }}</h2>
        <span class="spacer"></span>
        <button class="ghost" :title="t('common.close')" @click="styleSaveOpen = false"><AppIcon name="x" /></button>
      </div>
      <p class="muted">{{ t('my.save.body') }}</p>
      <div class="set-row">
        <input v-model="styleSaveName" :placeholder="t('my.save.name')" maxlength="80"
               @keyup.enter="styleSaveOpen = false; saveStyleToLibrary()" @keyup.esc="styleSaveOpen = false">
        <button class="primary" :title="t('my.save.tip')" @click="styleSaveOpen = false; saveStyleToLibrary()">{{ t('common.save') }}</button>
      </div>
    </div>
  </div>

</template>

<style>
/* Стили приложения — глобальные: экранные компоненты (components/) рендерятся
   внутри этого корня и пользуются теми же классами. */
/* раскладка окна: шапка сверху неподвижна, под ней .app-body со своей прокруткой */
html, body, #app { height: 100%; }
body { overflow: hidden; }
#app { display: flex; flex-direction: column; }
/* каркас: слева статичный блок (лого, статистика, очередь, меню), справа колонка
   из шапки, прокручиваемого контекста и плеера */
.app-shell { flex: 1 1 auto; min-height: 0; display: flex; }
/* правая колонка: содержимое в рамке со всех сторон, под ним плеер. Левая панель, полоса
   заголовка, плеер и края окна одного цвета (--frame), линия отделяет от них содержимое
   (а не обрывается у заголовка) */
.app-col { flex: 1 1 auto; min-width: 0; min-height: 0; display: flex; flex-direction: column;
  background: var(--frame); }
.app-body { flex: 1 1 auto; min-height: 0; overflow: auto; background: var(--bg);
  border: 1px solid var(--border); }
/* раздел в основном контексте: страница (PageView) прокручивается сама */
.ctx-view { flex: 1 1 auto; min-height: 0; overflow: auto; }
/* форма нового трека: слева блоки 1–3, справа «Отправка» — прилипает при прокрутке;
   на узком окне колонка «Отправка» уходит под форму */
/* поля звучания — в три колонки (раскладка страницы — PageView/PageBox) */
.page-box-body .slots { margin: 0; grid-template-columns: repeat(3, minmax(0, 1fr)); }
.nt-actions { display: flex; flex-direction: column; gap: 8px; }
.nt-submit { padding: 9px 16px; font-size: 15px; }
.nt-fan { display: flex; gap: 6px; align-items: center; }
.nt-fan > :first-child { flex: 0 0 72px; }
.nt-fan > button { flex: 1; }
header { flex: 0 0 auto; }
header {
  /* выше панелей main (стекинг-контексты из backdrop-filter), но ниже модалок (z-index 70) */
  /* одна строка при любой ширине окна (min 900): на узком ужимается второстепенное, плеер не переносится */
  position: relative; z-index: 5; flex-wrap: nowrap;
  display: flex; align-items: center; gap: 14px; padding: 10px 16px;
  background: color-mix(in srgb, var(--panel2) 78%, transparent);
  backdrop-filter: blur(14px) saturate(1.15);
  -webkit-backdrop-filter: blur(14px) saturate(1.15);
  border-bottom: 1px solid var(--bevel-lo);
  box-shadow: 0 1px 4px rgba(0,0,0,.35);
}
h1 {
  font-size: 16px; margin: 0; letter-spacing: 2px; text-transform: uppercase;
  color: var(--accent-text); text-shadow: 0 0 10px var(--lcd-glow);
}
h2 {
  font-size: 13px; margin: -16px -16px 12px; padding: 6px 12px;
  color: var(--text); text-transform: uppercase; letter-spacing: 1.5px;
  background: linear-gradient(180deg, var(--panel3), var(--panel2));
  border-bottom: 1px solid var(--bevel-lo);
  text-shadow: 1px 1px 0 rgba(0,0,0,.4);
}
.icon-btn {
  background: none; border: none; box-shadow: none; cursor: pointer;
  color: var(--muted); font-size: 15px; padding: 2px 0; font-weight: 400;
  /* все иконки шапки — одинаковые квадраты: ☀/☾, EN/RU, ⋮ */
  width: 30px; height: 26px; line-height: 22px; text-align: center;
}
.icon-btn:hover { color: var(--text); filter: none; transform: none; }
.recipe-progress { padding: 6px 12px; font-size: 13px; opacity: .85; }
.recipe-progress.error { color: var(--err); }
.compat-banner {
  display: flex; align-items: center; gap: 8px; padding: 6px 12px;
  background: color-mix(in srgb, var(--warn, #c90) 18%, transparent); font-size: 13px;
}
.compat-banner button { margin-left: auto; }
.health-dot {
  width: 10px; height: 10px; border-radius: 50%; flex: none;
  border: 1px solid var(--bevel-lo); cursor: help;
}
.health-dot.up { background: var(--ok); box-shadow: 0 0 8px var(--ok); }
.health-dot.down { background: var(--err); box-shadow: 0 0 8px var(--err); }
/* футер-плеер: под содержимым, на той же поверхности, что заголовок, левая панель и
   края окна (--frame) — часть окна, а не полоса поверх содержимого */
.playerbar-footer {
  position: relative; z-index: 5; flex: 0 0 auto;
  display: flex; align-items: center; padding: 6px 10px 0;
  background: var(--frame);
}
.playerbar-footer .playerbar { flex: 1 1 auto; min-width: 0; }
/* статус воркера в шапке — компактная строка после точки здоровья */
.head-stats {
  display: flex; align-items: center; gap: 8px; min-width: 0; flex: 0 1 auto;
  font-size: 11.5px; color: var(--muted);
}
.sb-text { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sb-vram { display: flex; align-items: center; gap: 6px; }
.sb-vram-bar {
  width: 70px; height: 5px; border-radius: 3px; overflow: hidden; flex: none;
  background: var(--bevel-lo);
}
.sb-vram-fill { display: block; height: 100%; background: var(--ok); border-radius: 3px; }
.sb-vram-fill.hot { background: var(--err); }
.settings-page { display: flex; justify-content: center; align-items: flex-start; }
.panel.lib { position: relative; z-index: 3; } /* выпадашки VSelect выше соседних панелей (стекинг-контексты из backdrop-filter) */
.panel { width: 640px; max-width: 100%; display: flex; flex-direction: column; gap: 12px; }
/* очередь — правая колонка сетки: растягивается на всё свободное место */
.track-list { width: auto; }
/* приёмы студии — группы по смыслу: подпись сверху, кнопки заворачиваются
   внутри своей карточки; фон плотный, группа читается на фоне секции */
.trick-bar { display: flex; flex-wrap: wrap; gap: 8px; align-items: stretch; margin-top: 0; }
.trick-group {
  display: flex; flex-direction: column; gap: 5px;
  padding: 6px 8px 7px; border: 1px solid var(--border); border-radius: 6px;
  background: var(--panel2);
}
.trick-cap {
  font-size: 10px; text-transform: uppercase; letter-spacing: .5px;
  color: var(--muted); user-select: none; white-space: nowrap;
}
.trick-btns { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.trick-btns .cont-style { min-width: 170px; flex: 1 1 220px; }
/* продолжение с места — строкой на всю ширину: поле тянется, подсказки строками по группам */
.trick-group.cont { flex-basis: 100%; }
.cont-hints { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; font-size: 12px; }
.cont-hints > .muted { min-width: 72px; }
.cont-cyr { font-size: 12px; margin: 0; color: var(--err); }
.trick-group.assemble { flex-basis: 100%; flex-direction: row; align-items: center; gap: 10px; }
.trick-group.assemble .trick-btns { flex: 1; }
.trick-hint { font-size: 11px; margin: 4px 0 0; }
/* живой отклик: пульсирующая нота, пока кусок генерится */
.pulse { display: inline-block; animation: trickpulse 1.2s ease-in-out infinite; }
@keyframes trickpulse { 0%, 100% { opacity: .35 } 50% { opacity: 1 } }
/* такты с приёмами: точка в углу + рамка акцентом, метка живёт и в выделении */
.roll-cell.trick { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); }
.roll-cell.trick::after { content: ''; position: absolute; top: 1px; right: 1px; width: 4px; height: 4px; border-radius: 50%; background: var(--accent); }
.plan-marks { font-size: 13px; margin: 0; color: var(--accent); }
.plan-marks .muted { display: block; font-size: 12px; }
.queue-tools { display: flex; gap: 8px; flex-wrap: wrap; align-items: center;
  /* прилипает к верху прокрутки: фильтры и поиск всегда под рукой, как волна в студии */
  position: sticky; top: 0; z-index: 3; background: var(--bg); padding: 6px 0; }
.queue-tools input { flex: 1 1 150px; min-width: 0; }
.queue-tools .queue-search { flex: 2 1 240px; }
/* фильтры: в широком окне — в строку с поиском; уже 1200 px — за кнопкой «Фильтры» */
.queue-filters { display: contents; }
.queue-filter-btn { display: none; flex: none; }
@media (max-width: 1200px) {
  .queue-filter-btn { display: inline-flex; }
  .queue-filters { display: none; }
  .queue-filters.open { display: flex; flex-wrap: wrap; gap: 8px; flex-basis: 100%; }
}
/* играющий трек: акцентная черта слева, номер — значком, название — акцентом */
.job.playing > .job-row { box-shadow: inset 3px 0 0 var(--accent); }
.job.playing .tl-num, .job.playing .job-name { color: var(--accent-text); }
.list-end { text-align: center; padding: 10px; font-size: 12px; }
.nav-wrap { position: relative; }
.nav-menu {
  position: absolute; top: 100%; right: 0; z-index: 40; margin: 2px 0 0; padding: 4px 0;
  list-style: none; background: var(--panel); border: 1px solid var(--border); border-radius: 6px;
  min-width: 175px; box-shadow: 0 8px 24px rgba(0,0,0,.4); white-space: nowrap;
}
.nav-menu li { padding: 6px 12px; font-size: 13px; cursor: pointer; display: flex; align-items: center; gap: 8px; }
.nav-menu li:hover { background: var(--panel2); }
/* колонка под иконку фиксированной ширины — подписи выровнены, эмодзи разной ширины не толкают текст */
.nav-ico { flex: none; width: 20px; text-align: center; }
.set-row { display: flex; align-items: center; gap: 10px; }
.set-row input, .set-row select { flex: 1; min-width: 0; width: 100%; }
/* галочки и переключатели в строке настроек не растягиваются в полосу */
.set-row input[type=checkbox], .set-row input[type=radio] { flex: none; width: auto; }
.slot-box { display: flex; flex-direction: column; position: relative; }
.slot-box > span { font-size: 11px; color: var(--muted); margin-bottom: 2px; }
.slot-drop {
  position: absolute; top: 100%; left: 0; right: 0; z-index: 30; margin: 2px 0 0; padding: 4px 0;
  list-style: none; background: var(--panel); border: 1px solid var(--border); border-radius: 6px;
  max-height: 240px; overflow-y: auto; box-shadow: 0 8px 24px rgba(0,0,0,.4);
}
.slot-drop li { padding: 4px 10px; font-size: 12px; cursor: pointer; }
.slot-drop li:hover { background: var(--panel2); }
.lib-actions { margin-top: 8px; display: flex; gap: 8px; }
.lib-row { display: flex; gap: 6px; align-items: center; }
.lib-row > :first-child { flex: 1.1; min-width: 0; }
.lib-row > :nth-child(2) { flex: 1.6; min-width: 0; }
.lib-row .small-btn { flex: none; padding: 4px 8px; }
.lib .select, .lib select { width: 100%; }
.lib-group { margin-bottom: 10px; }
.lib-group-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.lib-group-items { font-size: 11px; line-height: 1.5; margin-top: 2px; }
.lib-style-row { display: flex; align-items: center; gap: 8px; font-size: 13px; padding: 2px 0; }
.lib-name-input { max-width: 200px; font-size: 13px; padding: 3px 8px; }
.lib-style-text { flex: 1; font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.set-label { width: 110px; flex: none; font-size: 12px; color: var(--muted); }
.set-hint { font-size: 12px; }
.set-info { font-size: 11px; line-height: 1.6; word-break: break-all; }
.set-actions { display: flex; gap: 8px; margin-top: 8px; }
.ok { color: var(--ok); font-size: 12px; }
/* плеер — гибкая часть шапки: всё место между левыми кнопками и «＋ Новый трек» */
/* узкое окно: меньше промежутки, короче громкость и название, у «＋ Новый трек» — только «＋» */
@media (max-width: 1150px) {
  header { gap: 8px; padding: 8px 12px; }
  .playerbar-footer .playerbar { gap: 5px; }
  .playerbar-footer .playerbar .vol input { width: 50px; }
  h1 { letter-spacing: 1px; }
}
@media (max-width: 1000px) {
  .new-track-txt { display: none; }
  .new-track-btn { padding: 6px 12px; font-size: 16px; }
}
.new-track-btn { margin-left: auto; }
.playerbar { display: flex; align-items: center; gap: 8px; font-size: 12px; }
/* ▶ — главная кнопка плеера: круглая, янтарная (--play-grad), крупнее соседних */
.playerbar .player-play { width: 34px; height: 34px; min-width: 0; padding: 0; border-radius: 50%;
  display: flex; align-items: center; justify-content: center; }
.playerbar .player-play svg { width: 12px; height: 12px; }
/* полоса прокрутки растягивается на свободное место шапки; под ней —
   метки минут-секунд: плеер здесь — навигация по треку, не просто индикатор */
.playerbar .seek-wrap { flex: 1 1 auto; min-width: 80px; display: flex; flex-direction: column; }
.playerbar .seek { width: 100%; }
.seek-ticks { position: relative; height: 11px; margin-top: -1px; }
.seek-tick {
  position: absolute; transform: translateX(-50%); top: 0;
  font-size: 9px; color: var(--muted); font-variant-numeric: tabular-nums;
  pointer-events: none; user-select: none;
}
.playerbar .vol { display: flex; align-items: center; gap: 4px; color: var(--muted); }
.playerbar .vol input { width: 80px; }
/* длинное бежит строкой (две копии подряд, сдвиг на половину — без шва) */
/* ширина по названию (не больше 220 px, длиннее — бегущая строка): короткое «#523» не оставляет дыры */
.playerbar .now { flex: 0 1 auto; max-width: 220px; min-width: 0; overflow: hidden; white-space: nowrap; }
.playerbar .now-track { display: inline-flex; }
.playerbar .now-track.scroll { animation: now-marquee linear infinite; }
.playerbar .now-track.scroll .now-text { padding-right: 3em; }
.playerbar .now:hover .now-track { animation-play-state: paused; }
@keyframes now-marquee { from { transform: translateX(0); } to { transform: translateX(-50%); } }
@media (prefers-reduced-motion: reduce) {
  .playerbar .now-track.scroll { animation: none; }
  .playerbar .now { text-overflow: ellipsis; }
}
.playerbar .pos {
  font-variant-numeric: tabular-nums; white-space: nowrap; flex: none;
  /* ширина под самую длинную надпись («10:00 / 10:00»): «→ 1:23» при наведении
     короче — без фиксированной ширины полоса прокрутки рядом прыгала */
  width: 13ch; box-sizing: content-box; text-align: center;
  background: var(--lcd-bg); color: var(--lcd-text);
  font-family: 'DejaVu Sans Mono', 'Consolas', monospace;
  padding: 2px 8px; border-radius: 2px; border: 1px solid var(--border);
  box-shadow: inset 0 0 6px rgba(0,0,0,.55); text-shadow: 0 0 6px var(--lcd-glow);
}
/* светлая: экран таймера светлый — тёмная внутренняя тень была бы грязью */
[data-theme="light"] .playerbar .pos { box-shadow: inset 0 1px 2px rgba(15,20,35,.1); }
.playerbar .ghost {
  min-width: 30px; width: auto; height: 24px; padding: 0 4px;
  display: inline-flex; align-items: center; justify-content: center;
  color: var(--text);
  background: linear-gradient(180deg, var(--panel3), var(--panel2));
  border: 1px solid var(--border);
  border-top-color: var(--bevel-hi);
  border-left-color: var(--bevel-hi);
  font-size: 12px; line-height: 1;
}
.playerbar .ghost:hover { color: var(--accent-text); border-color: var(--accent-text); }
.spacer { flex: 1; }

main {
  display: grid;
  /* одна страница за раз: список треков или форма нового трека */
  grid-template-columns: minmax(0, 1fr);
  /* поля задаёт сама страница (PageView) */
  max-width: 1800px; margin: 0 auto;
}
/* тело модалки прокручивается само: части формы не сжимаются (иначе стих — в одну строку) */
.home-link { cursor: pointer; display: inline-flex; align-items: center; gap: 8px; white-space: nowrap; }
/* значок приложения в шапке — тот же, что иконка окна (build/appicon.png) */
.app-logo { width: 26px; height: 26px; flex: none; filter: drop-shadow(0 0 4px var(--lcd-glow)); }
.new-track-btn { font-weight: 600; padding: 6px 14px; }
.new-track-btn.on { outline: 2px solid var(--accent, currentColor); outline-offset: 1px; }
.left-col { display: flex; flex-direction: column; gap: 20px; min-width: 0; }
.left-col input, .left-col textarea { min-width: 0; max-width: 100%; }
.panel {
  background: color-mix(in srgb, var(--panel) 88%, transparent);
  backdrop-filter: blur(14px) saturate(1.12);
  -webkit-backdrop-filter: blur(14px) saturate(1.12);
  border-radius: 2px; padding: 16px;
  border: 1px solid var(--border);
  border-top-color: var(--bevel-hi); border-left-color: var(--bevel-hi);
  border-bottom-color: var(--bevel-lo); border-right-color: var(--bevel-lo);
  box-shadow: inset 1px 1px 0 rgba(255,255,255,.05), 0 2px 6px rgba(0,0,0,.25);
}

.ghost.icon.del { color: var(--muted); }
.ghost.icon.del:hover { color: var(--err); }
.primary.danger { background: var(--err); }
.confirm-modal { width: min(480px, 92vw); }

.slots { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px 12px; margin: 12px 0; }
.slots label { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--muted); }
.slots input { font-size: 13px; }

details { margin: 8px 0; color: var(--muted); font-size: 13px; }
details textarea { width: 100%; margin-top: 6px; }

textarea { width: 100%; resize: vertical; font-family: inherit; }

.row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 10px 0 6px; font-size: 12px; }
.lyrics-tools { display: flex; gap: 6px; align-items: center; }
.lyrics-tools .dur-select { max-width: 110px; font-size: 12px; }
.lyrics-tags { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; margin: 0 0 4px; }
.lyrics-tags-hint { font-size: 11px; }
button.toggle { background: transparent; border: 1px solid var(--border); color: var(--muted); font-weight: 400; font-size: 12px; padding: 3px 10px; }
button.toggle.on { border-color: var(--accent); color: var(--accent); font-weight: 600; }
.seed input { width: 110px; min-width: 0; }
.arc-row { align-items: center; gap: 6px; }
.arc-title { font-size: 12px; color: var(--muted); }
.character-row { align-items: center; gap: 12px; flex-wrap: wrap; }
.character-knob { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; }
.character-knob input[type=range] { width: 110px; }
.character-val { min-width: 2.5em; font-variant-numeric: tabular-nums; }
.form-advanced > summary { cursor: pointer; font-size: 13px; color: var(--muted); padding: 4px 0; }
.form-advanced[open] > summary { margin-bottom: 6px; }
.cot-radios { display: flex; align-items: center; gap: 10px; font-size: 12px; color: var(--muted); }
.cot-radios label { display: flex; align-items: center; gap: 4px; cursor: pointer; }
.cot-title { font-weight: 600; }
.style-out { align-items: center; gap: 8px; flex-wrap: nowrap; }
.style-out-cap { flex: none; font-size: 12px; color: var(--muted); }
.compiled { flex: 1; min-width: 0; font-size: 12px; color: var(--text); margin: 0; word-break: break-word; }
.style-cyr { font-size: 12px; color: var(--err); margin: -2px 0 6px; }

.primary {
  background: var(--play-grad);
  color: var(--play-fg); padding: 6px 16px; text-shadow: none;
}
.primary.alt {
  background: linear-gradient(180deg, var(--panel3), var(--panel2));
  border: 1px solid var(--lcd-text); color: var(--lcd-text);
}
/* светлая: янтарная обводка alt-кнопки на белом блёклая — акцент темы */
[data-theme="light"] .primary.alt { border-color: var(--accent); color: var(--accent); }

.progress-wrap { display: inline-flex; align-items: center; gap: 8px; min-width: 180px; }
.progress-track { display: inline-block; width: 110px; height: 8px; border-radius: 4px;
  background: var(--panel2); border: 1px solid var(--border); overflow: hidden; position: relative; }
.progress-fill { display: block; height: 100%;
  background: linear-gradient(90deg, var(--run), var(--ok)); transition: width 1s linear; }
.progress-track.indet::after { content: ''; position: absolute; inset: 0 auto 0 -30%; width: 30%;
  background: var(--run); border-radius: 4px; animation: progress-slide 1.2s ease-in-out infinite; }
@keyframes progress-slide { to { left: 100%; } }
.progress-label { font-size: 11px; white-space: nowrap; }

/* треклист — таблица как в музыкальных программах: шапка столбцов, строки одной сетки (ширины .col-*),
   под названием — короткий стиль; разделитель — тонкая линия, раскрытый трек отмечен только фоном */
.job-table { margin-bottom: 10px; }
.job { display: flex; flex-direction: column; }
.job > .job-row { border-bottom: 1px solid color-mix(in srgb, var(--border) 55%, transparent); }
.job-row { display: flex; align-items: center; gap: 12px; padding: 8px 8px; cursor: pointer; user-select: none; min-width: 0; }
.job-row:hover { background: color-mix(in srgb, var(--panel2) 70%, transparent); }
.job.open > .job-row { background: var(--panel2); }
.job-head-row { cursor: default; padding-top: 2px; padding-bottom: 6px; font-size: 11px; text-transform: uppercase;
  letter-spacing: .4px; color: var(--muted); border-bottom: 1px solid var(--border); }
.job-head-row:hover { background: none; }
.job-main { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
.job-title-line { display: flex; align-items: center; gap: 8px; min-width: 0; }
.job-title-line > :not(.job-name) { flex: none; }
.job-style-line { font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.col-kids { flex: none; width: 56px; display: flex; justify-content: center; }
.col-folder { flex: none; width: 110px; display: flex; min-width: 0; }
.col-folder .tl-pill { overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
.col-dur { flex: none; width: 44px; text-align: right; }
.col-when { flex: none; width: 92px; text-align: right; }
.col-actions { flex: none; width: 180px; display: flex; align-items: center; justify-content: flex-end; gap: 6px; }
.col-actions > button, .col-actions .menu-pop > summary { white-space: nowrap; height: 28px; padding: 0 10px; font-size: 12px;
  display: inline-flex; align-items: center; }
.col-actions > .job-caret { padding: 0 6px; }
.job-head-row .col-dur, .job-head-row .col-when { font: inherit; }
/* узкое окно: сначала прячется дата, потом папка — название с кнопками не толкается */
@media (max-width: 1250px) { .col-when { display: none; } }
@media (max-width: 1100px) { .col-folder { display: none; } }
.job-caret { color: var(--muted); flex: none; }
.job-row:hover .job-caret { color: var(--text); }
.job.open .job-caret { color: var(--accent); }
.tl-num {
  font: 500 12.5px 'IBM Plex Mono', 'DejaVu Sans Mono', 'Consolas', monospace;
  color: var(--muted); flex: none; width: 34px; text-align: right;
}
.tl-play-cell { flex: none; width: 32px; height: 32px; display: flex; align-items: center; justify-content: center; }
/* круглая play-кнопка: контур, заливается акцентом при наведении/воспроизведении */
.tl-play {
  width: 30px; height: 30px; border-radius: 50%; padding: 0;
  border: 1px solid var(--border); background: transparent; color: var(--accent);
  display: flex; align-items: center; justify-content: center; cursor: pointer;
}
.tl-play:hover, .tl-play.is-playing { background: var(--accent); border-color: var(--accent); color: #fff; }
.tl-play:disabled { opacity: .5; cursor: default; }
.job-name { font-weight: 600; font-size: 15px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; flex: 0 1 auto; }
.job-progress { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
.progress-track.slim { width: 76px; height: 5px; flex: none; }
/* мета справа — моноширинная, приглушённая, не спорит с названием */
.tl-meta { font: 12.5px 'IBM Plex Mono', 'DejaVu Sans Mono', 'Consolas', monospace; color: var(--muted); white-space: nowrap; flex: none; }
.tl-pill {
  font: 12px 'IBM Plex Mono', 'DejaVu Sans Mono', 'Consolas', monospace; color: var(--muted);
  border: 1px solid var(--border); border-radius: 999px; padding: 2px 9px; flex: none; white-space: nowrap;
}
.tl-pill:hover { color: var(--accent); border-color: var(--accent); }
.kids-badge { cursor: pointer; }
/* точка-статус вместо чипа, когда играть нельзя */
.status-mini { width: 8px; height: 8px; border-radius: 50%; flex: none; background: var(--muted); }
.status-mini.running { background: var(--run); animation: trickpulse 1.2s ease-in-out infinite; }
.status-mini.error { background: var(--err); }
.status-mini.queued { background: var(--muted); }
/* разворот: продолжение строки на том же фоне, отступ — по колонке названия (№ + ▶) */
/* версии — от колонки ▶ трека (8 поля + 34 № + 12), остальное раскрытие — от колонки названия */
.job-detail { display: flex; flex-direction: column; min-width: 0; padding: 0 8px 10px 54px; background: var(--panel2);
  border-bottom: 1px solid var(--border); }
.job-detail > :not(.job-kids) { margin-left: 44px; }
.job-detail .job-meta-extra { display: flex; align-items: center; gap: 6px 10px; flex-wrap: wrap;
  font-size: 12px; color: var(--muted); margin-left: 10px; min-width: 0; }
/* мета-строка разворота: слева сид/характер, справа папка и обслуживание */
.job-meta-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  padding: 8px 12px 0; font-size: 12px; }
.job-row .job-rename { flex: none; color: var(--muted); opacity: 0; }
.job-row:hover .job-rename, .job.open .job-rename { opacity: 1; }
.title-edit.job-name { font-weight: 600; font-size: 15px; }
/* стиль в развороте — просто текст; клик разворачивает таблицу тегов */
.job-style { padding: 6px 12px 2px; }
.job-style .style-tags { cursor: pointer; }
/* звезда основной версии в списке версий: залитая с фоном у head, серый контур у остальных */
.kid-star {
  width: 26px; height: 26px; border-radius: 50%; padding: 0; flex: none;
  border: 1px solid var(--border); background: transparent; color: var(--muted);
  display: flex; align-items: center; justify-content: center; cursor: pointer;
}
.kid-star:hover { color: var(--accent2, var(--run)); border-color: var(--accent2, var(--run)); }
.kid-star.head { background: var(--accent); border-color: var(--accent); color: #fff; cursor: default; }
.job-body { padding: 4px 12px 8px; }
/* пока открыт хоть один трек, закрытые строки приглушены — взгляд держится на открытом */
.job-table.has-open .job:not(.open) { opacity: .6; transition: opacity .15s; }
.job-table.has-open .job:not(.open):hover { opacity: 1; }
.collapse-all { margin-left: 8px; }
.status { font-size: 12px; padding: 2px 8px; border-radius: 10px; background: var(--border); }
.status.done { background: rgba(255,190,61,.16); color: var(--lcd-text); text-shadow: 0 0 8px var(--lcd-glow); }
/* светлая: чип «готово» — акцентный текст на бледной подложке акцента */
[data-theme="light"] .status.done { background: color-mix(in srgb, var(--accent) 10%, transparent); }
.status.running { background: rgba(217,160,61,.2); color: var(--run); }
.status.error { background: rgba(224,93,61,.2); color: var(--err); }
/* выпадающее меню на <details>: кнопка-summary, список — как меню в шапке */
.menu-pop { position: relative; }
.menu-pop > summary { list-style: none; cursor: pointer; }
.menu-pop > summary::-webkit-details-marker { display: none; }
.menu-pop > summary.ghost-btn {
  display: inline-block; padding: 4px 10px; font-size: 12px; color: var(--muted); user-select: none;
  background: linear-gradient(180deg, var(--panel2), var(--panel));
  border: 1px solid var(--btn-border); border-top-color: var(--btn-bevel-hi);
}
.menu-pop > summary.ghost-btn:hover, .menu-pop[open] > summary.ghost-btn { color: var(--text); filter: brightness(1.1); }
.menu-pop > .nav-menu { top: auto; bottom: 100%; margin: 0 0 2px; }
/* меню вниз (в строке заголовка страницы): список под кнопкой, по правому краю */
.menu-pop.down > .nav-menu { top: 100%; bottom: auto; left: auto; right: 0; margin: 2px 0 0; }
.menu-pop > summary.ghost-btn.busy { opacity: .6; }
.menu-pop .nav-menu li.danger { color: var(--err, #e5534b); }
.style-tags { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 6px 0; align-items: baseline; }
.style-tag-key {
  font-size: 11px; text-transform: uppercase; letter-spacing: .4px;
  color: var(--muted); user-select: none; white-space: nowrap;
}
.style-tag-val { font-size: 12px; color: var(--muted); min-width: 0; }
.error { color: var(--err); font-size: 13px; }
.job .play-main { flex: none; }
.muted { color: var(--muted); }
.badge.draft { background: var(--badge-bg); color: var(--badge-fg); font-style: italic; }
.badge { font-size: 11px; padding: 1px 7px; border-radius: 9px; background: var(--badge-bg); color: var(--badge-fg); }
.play-main {
  background: var(--play-grad);
  color: var(--play-fg); padding: 6px 16px; min-width: 44px;
  text-shadow: none;
}
.ghost.icon { padding: 2px 8px; font-size: 13px; }

/* диалоги (план/метрики/подтверждение) — выше страниц-модалок (z-index 60):
   страница-модалка сама есть modal-backdrop+page-backdrop, и page-backdrop ниже в файле возвращает ей 60 */
.modal-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,.55); display: flex; align-items: center; justify-content: center; z-index: 70; }
.modal { background: var(--panel); border: 1px solid var(--border); border-radius: 12px; padding: 18px 20px; width: min(760px, 92vw); max-height: 90vh; display: flex; flex-direction: column; gap: 10px; }
/* страницы-модалки (настройки/свои треки/голоса) поверх основного контента:
   закрываются ✕, Esc и кликом по фону — кнопка «Вернуться» не нужна */
.page-backdrop { z-index: 60; padding: 24px; }
.page-modal { width: min(1100px, 96vw); max-width: 96vw; max-height: calc(100vh - 48px); overflow: hidden; }
.page-modal-head {
  display: flex; align-items: center; gap: 10px;
  margin: -16px -16px 0; padding: 6px 12px;
  background: linear-gradient(180deg, var(--panel3), var(--panel2));
  border-bottom: 1px solid var(--bevel-lo);
}
.page-modal-head h2 { margin: 0; padding: 0; background: none; border: none; flex: 1; }
/* боковой паддинг тела = паддингу панели: внутренние h2 (поля −16px для стыковки
   с панелью) ложатся вровень с шапкой и не выпирают горизонтальным скроллом */
.page-modal-body { overflow-y: auto; min-height: 0; display: flex; flex-direction: column; gap: 12px; padding: 12px 16px 8px; }
.modal-head { display: flex; align-items: baseline; gap: 12px; }
.modal-head h2 { margin: 0; }
.plan-meta { font-size: 12px; }
.abc { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; line-height: 1.45; }
.abc-row { display: flex; align-items: center; gap: 8px; margin-top: 8px; }
.submit-err { max-width: 40%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; align-self: center; }
.modal-actions { display: flex; gap: 8px; }

.metrics-modal { width: min(640px, 92vw); }
.cmp-row { display: flex; align-items: center; gap: 10px; font-size: 13px; flex-wrap: wrap; }
.cmp-row select { max-width: 320px; }
.metrics-table { display: flex; flex-direction: column; gap: 2px; font-size: 13px; }
.mrow { display: grid; grid-template-columns: 1.6fr 1fr 1fr .8fr; gap: 8px; padding: 3px 6px; border-radius: 4px; }
.mrow:nth-child(odd) { background: var(--panel2); }
.mrow.head { color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .5px; }
.mrow span:not(:first-child) { text-align: right; font-variant-numeric: tabular-nums; }
.mrow.head span:first-child { text-align: left; }
.delta { color: var(--run); }
.delta:empty { color: transparent; }

.dsp-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.dsp-row select { max-width: 220px; }
.dsp-note { font-size: 12px; margin: 0; }
.dsp-params { display: flex; flex-direction: column; gap: 4px; }
.dsp-params label { display: grid; grid-template-columns: 190px 1fr 52px; align-items: center; gap: 10px; font-size: 12px; color: var(--muted); }
.dsp-params input[type="range"] { width: 100%; }
.dsp-pval { text-align: right; font-variant-numeric: tabular-nums; color: inherit; }
.dsp-variant { display: flex; align-items: center; gap: 10px; font-size: 12px; flex-wrap: wrap; }
.dsp-variant .deltas { font-variant-numeric: tabular-nums; }
.play-mini { padding: 2px 9px; }
.play-mini.stop { color: var(--err); font-weight: bold; }
.primary.small { padding: 4px 12px; font-size: 12px; }

.small-btn { font-size: 12px; padding: 3px 10px; }
.cop-instruction { margin: 10px 0; font-size: 12px; }
.cop-instruction summary { cursor: pointer; color: var(--muted); margin-bottom: 4px; }
.cop-instruction textarea { width: 100%; }
.cop-modal { width: min(620px, 92vw); }
.cop-form { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.cop-form input[type="text"], .cop-form input:not([type]) { flex: 1; min-width: 220px; }
.cop-example { font-size: 12px; color: var(--muted); display: flex; align-items: center; gap: 5px; }


.corpus-new { display: flex; gap: 8px; }
.corpus-new input { flex: 1; }
.corpus-item { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; border-top: 1px solid var(--border); padding: 8px 0 4px; margin-top: 8px; font-size: 13px; }
.corpus-actions { display: flex; gap: 8px; }
.voice-hint { font-size: 12px; margin: 6px 0 0; }
.voice-presets { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin: 8px 0 10px; font-size: 12px; }
.voice-preview { font-size: 12px; margin: 6px 0 0; word-break: break-word; }
.voice-cyr { font-size: 12px; margin: 4px 0 0; color: var(--err); }
.voice-card-desc { flex: 1 1 240px; min-width: 0; word-break: break-word; font-size: 12px; }

/* контейнер секций студии.
   min-width: 0 — иначе минимальная ширина ролл-сетки (сотни тактов) растягивает
   студию шире окна вместо прокрутки внутри roll-scroll */
/* clip, а не auto: auto делал блок контейнером прокрутки, и «волна сверху» (sticky) прилипала к нему, а не к странице;
   широкий ролл прокручивается своим .roll-scroll */
.roll-block { margin-top: 8px; padding: 0; min-width: 0; overflow-x: clip; }
.roll-voices { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-bottom: 10px; font-size: 12px; }
.studio-page { justify-content: stretch; }
/* студия — без общей панели (секции-блоки лежат прямо на фоне страницы,
   не «форма в форме»); заголовок — строкой, без плашки h2 */
.studio-page .studio-sheet { width: auto; max-width: none; flex: 1; margin: 0 16px; min-width: 0; padding-bottom: 12px; }
.studio-sheet > h2 {
  margin: 8px 2px 2px; padding: 0; background: none; border: none; text-shadow: none;
}
/* «вернуться» из студии в шапке: название трека рядом, длинное — многоточием */
.studio-back { flex: none; }
.studio-back-title { max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* полоса шагов прилипает к верху прокрутки; высота фиксирована, отступов нет —
   под ней без щелей встаёт прилипшая волна (margin не красится, сквозь него
   просвечивало содержимое) */
.studio-steps {
  position: sticky; top: 0; z-index: 4; display: flex; gap: 6px; align-items: center;
  height: 46px; box-sizing: border-box; padding: 0 2px; margin: 0;
  flex-wrap: nowrap; overflow-x: auto; background: var(--bg);
}
.studio-steps .on { color: var(--lcd-text); border-color: var(--lcd-text); }
.studio-title { margin-left: auto; min-width: 0; overflow: hidden; text-overflow: ellipsis;
  white-space: nowrap; font-size: 13px; flex: none; max-width: 40%; }
/* активная дорожка студии — плашка в полосе шагов */
.active-stem { display: inline-flex; align-items: center; gap: 6px; margin-left: 8px; padding: 2px 8px;
  border: 1px solid var(--lcd-text); border-radius: 12px; font-size: 12px; }
[data-theme="light"] .active-stem { border-color: var(--accent); }
/* волна прилипает вплотную под полосой шагов: видна из любого шага (галочка «волна сверху»).
   Фон — сплошная панель: полупрозрачный studio-box просвечивал */
.wave-panel.wave-pinned { position: sticky; top: 46px; z-index: 3; margin-top: 0; background: var(--panel); }
/* в светлой теме янтарь ЖК на светлом не читается — активный шаг акцентным цветом */
[data-theme="light"] .studio-steps .on { color: var(--accent); border-color: var(--accent); }
.studio-step { margin: 22px 2px 8px; font-size: 14px; letter-spacing: 1px; text-transform: uppercase; scroll-margin-top: 56px; }
.studio-step .muted { text-transform: none; letter-spacing: 0; font-size: 12px; font-weight: 400; }
/* секции студии (звук/план/правки/вклейки/стемы/овердаб/эффекты) — рамка
   со шапкой с фоном, как заголовок карточки трека */
.studio-box {
  border: 1px solid var(--border); border-radius: 6px; margin-top: 12px;
  overflow: hidden; background: color-mix(in srgb, var(--panel) 70%, transparent);
}
.studio-box-head {
  display: flex; align-items: center; gap: 10px; padding: 6px 10px;
  background: var(--panel); border-bottom: 1px solid var(--border);
  box-shadow: inset 1px 1px 0 rgba(255,255,255,.05);
  font-size: 12px; font-weight: 600; letter-spacing: .3px; text-transform: uppercase; color: var(--muted);
}
.studio-box-hint { font-weight: 400; text-transform: none; letter-spacing: 0; font-size: 11px; }
.studio-box-tools { margin-left: auto; display: inline-flex; gap: 4px; align-items: center; }
.studio-box-body { padding: 8px 10px; }
/* раскрываемые секции (овердаб, эффекты): стрелка вместо маркера details */
details.studio-box > summary { cursor: pointer; list-style: none; }
details.studio-box > summary::-webkit-details-marker { display: none; }
details.studio-box > summary::before { content: '▸'; color: var(--muted); transition: transform .15s; }
details.studio-box[open] > summary::before { transform: rotate(90deg); }
details.studio-box[open] > .studio-box-head { margin-bottom: 0; }
/* шаг студии («Звук», «Голос», «Готово») одной рамкой, его блоки — карточками внутри, как группы приёмов
   в «Править»: глазу проще разделять шаги и блоки */
.studio-step-box { padding: 8px; display: flex; flex-direction: column; gap: 8px; }
.studio-step-box > .studio-box, .studio-step-box > details.studio-box { margin-top: 0; background: var(--panel2); }
.studio-step-box > .studio-box > .studio-box-head { background: transparent; box-shadow: none; }
.studio-step-box > .quick-fx-head { margin: 4px 2px 0; }
.abc-help { margin-bottom: 8px; font-size: 12px; }
.abc-help summary { cursor: pointer; color: var(--muted); }
.abc-help p { margin: 6px 0 0; line-height: 1.6; }
.roll-voice-edit { display: flex; align-items: center; gap: 4px; }
.roll-meta { margin: 0 0 8px; font-size: 12px; }
.roll-grid { display: grid; gap: 2px; font-size: 10px; user-select: none; }
.roll-grid + .roll-grid { margin-top: 8px; }
.roll-voice { font-size: 10px; color: var(--muted); white-space: nowrap; overflow: hidden; }
.roll-sec { font-size: 9px; color: var(--muted); white-space: nowrap; overflow: visible; }
/* начало секции: подпись целиком (поверх соседних пустых ячеек) и метка-граница */
.roll-sec.start { border-left: 1px solid var(--border); padding-left: 2px; position: relative; z-index: 1; }
.roll-cell { height: 18px; border-radius: 3px; cursor: pointer; border: 1px solid transparent; position: relative; }
.roll-cell.d0 { background: var(--panel); }
.roll-cell.d1 { background: rgba(120,140,255,.18); }
.roll-cell.d2 { background: rgba(120,140,255,.42); }
.roll-cell.d3 { background: rgba(120,140,255,.72); }
.roll-cell.sel { border-color: var(--ok); box-shadow: 0 0 0 1px var(--ok); }
.roll-cell.off { opacity: .3; }
.roll-chord { font-size: 9px; color: var(--muted); text-align: center; overflow: hidden; }

/* волна громкости: сетка/выделение/курсор рисует
   canvas, спектрограмма — <img> воркера под ним, ось X у обоих 0..длительность */
/* студия: секции-блоки .studio-box выше; зум ролла — вправо в шапке секции */
.roll-zoom { margin-left: auto; display: inline-flex; gap: 4px; }
.wave-toolbar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; font-size: 12px; }
/* выбор цвета волны: кружки-свотчи, активный — с обводкой */
.wave-colors { display: inline-flex; align-items: center; gap: 5px; }
.wave-swatch {
  width: 15px; height: 15px; min-width: 15px; padding: 0; border-radius: 50%;
  border: 1px solid var(--border); cursor: pointer;
}
.wave-swatch.on { outline: 2px solid var(--text); outline-offset: 1px; }
.wave-toolbar .ghost.on { border-color: var(--accent); color: var(--accent); font-weight: 600; }
.wave-snap { display: inline-flex; align-items: center; gap: 4px; color: var(--muted); cursor: pointer; }
.wave-hint { margin-left: auto; font-weight: 400; text-transform: none; letter-spacing: 0; }
.wave-wrap { position: relative; height: 96px; border: 1px solid var(--border); border-radius: 8px; background: var(--panel2); overflow: hidden; }
.wave-spectrum { position: absolute; top: 0; height: 100%; object-fit: fill; }
.wave-empty { position: absolute; inset: 0; }
.wave-canvas { position: absolute; inset: 0; width: 100%; height: 100%; cursor: crosshair; touch-action: none; }
.wave-zoom { position: absolute; top: 4px; right: 6px; display: inline-flex; gap: 4px; align-items: center; font-size: 11px; }
/* полоса прокрутки окна волны нарисована (волна не прокручивается как содержимое), но выглядит
   как общий скроллбар из style.css: ползунок 8 px на прозрачной дорожке, на наведении — акцент */
.wave-scroll { position: relative; height: 10px; margin-top: 3px; cursor: grab; user-select: none; touch-action: none; }
.wave-thumb { position: absolute; top: 1px; bottom: 1px; border-radius: 4px; background: var(--scroll-thumb); }
.wave-scroll:active .wave-thumb, .wave-scroll:hover .wave-thumb { background: var(--accent); }

/* прокрутка ролла: только вокруг сетки тактов — скроллбар прямо под ней (вид — общий, style.css);
   отступ до него — как у полосы под волной (.wave-scroll margin-top) */
.roll-scroll { overflow-x: auto; padding-bottom: 3px; }

.od-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.od-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.od-style { flex: 1; min-width: 240px; }
.od-gain { font-size: 12px; color: var(--muted); display: flex; align-items: center; gap: 6px; }

.stem-row { display: flex; align-items: center; gap: 10px; font-size: 13px; }
</style>
