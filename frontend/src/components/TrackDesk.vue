<script setup>
// Пульт дорожек студии: строка на каждую дорожку трека — послушать её, видеть её правки и собрать ей звук.
// Раскладка строк и окна — чистый модуль trackDesk.js.
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { useInserts } from '../composables/useInserts.js'
import { usePlayer } from '../composables/usePlayer.js'
import { useWindowPlay } from '../composables/useWindowPlay.js'
import ChainEditor from './ChainEditor.vue'
import PlaceControls from './PlaceControls.vue'
import VSelect from '../VSelect.vue'
import BLOCKS from '../fxBlocks.json'
import { fxPresets } from '../fxPresets.js'
import { useInstruments } from '../composables/useInstruments.js'
import { installKit as installKitOn } from '../kitProgress.js'
import { fromWorkerChain, toWorkerChain, missingRequired, fillAmp } from '../fxChain.js'
import { instrumentTree, familyOptions, subOptions, itemOptions, pathOf } from '../presetGroups.js'
import { DRUM_KITS, DRUM_TREATMENTS } from '../drumKits.js'
import { applyEngine, ensureKits } from '../engineRun.js'
import { insertTitle, insertWindow } from '../insertLabels.js'
import { deskRows, previewWindow, applyWindow, presetsFor, removeRecord, rhythmSection, shouldAutoSplit, stemLevels, stemAudibility, withPending } from '../trackDesk.js'
import { notesHint } from '../guitarNotesHint.js'
import { stemPlace } from '../mixDesk.js'

const props = defineProps({
  job: { type: Object, required: true },
  stems: { type: Array, default: () => [] },     // дорожки трека [{name, file}]
  stemsLoaded: { type: Boolean, default: false }, // список дорожек прочитан (пустой — дорожек правда нет)
  sel: { type: Object, default: null },          // выделение на волне {from, to}
  cursor: { type: Number, default: 0 },          // курсор волны, с
  names: { type: Object, required: true },       // подписи записей реестра (как в «Изменениях трека»)
  win: { type: Object, required: true },         // подписи окон записей
  active: { type: String, default: '' },         // выбранная дорожка: волна/спектр и правки громкости — по ней
})
// applied — трек пересобран; stems — дорожки сделаны (студии перечитать список); select — выбрана дорожка
const emit = defineEmits(['applied', 'stems', 'select'])
const { t, locale } = useI18n()
const inserts = useInserts()
const { toggleArtifact, playBtn, isPlaying, playBusy } = usePlayer()
// готовые цепочки и свои инструменты (страница «Инструменты»): свой — внизу своей группы с пометкой «(мой)»
const { all: instruments } = useInstruments()
const before = useWindowPlay('desk-before', () => t('instr.before'))
const rowAfter = useWindowPlay('desk-row-after', () => t('instr.after'))
const rowBefore = useWindowPlay('desk-row-before', () => t('instr.before'))
const rhythmAfter = useWindowPlay('desk-rhythm-after', () => t('desk.rhythm.listen'))

const open = ref('')             // раскрытая дорожка
const fine = ref(false)          // «тонко настроить»: редактор блоков (по умолчанию — только выбор инструмента)
const familyId = ref('')         // выбор «семейство → подвид → инструмент» (как на странице «Инструменты»)
const groupId = ref('')
const editing = ref(null)        // запись движка на правке: {childId, from, to} | null
const chain = ref([])
const presetId = ref('')
const solo = ref(false)
const assets = ref({ amps: [], irs: [], kits: [] })
const busy = ref('')
const msg = ref('')
const err = ref('')

const tr = (lbl) => (lbl && (lbl[locale.value] || lbl.ru)) || ''
const rows = computed(() => deskRows(props.stems.map((s) => s.name), inserts.appliedFor(props.job.id)))
// разделено без частей барабанов (Demucs / старое разделение) — предложить разделить подробнее
const noParts = computed(() => props.stems.length > 0 && !props.stems.some((s) => s.name === 'kick'))
const fileOf = (stem) => (props.stems.find((s) => s.name === stem) || {}).file || `stem-${stem}.flac`
const stemName = (s) => t('studio.dsp.target.' + s)
const missing = computed(() => missingRequired(chain.value, BLOCKS))
const ready = computed(() => !busy.value && !missing.value.length && toWorkerChain(chain.value).length > 0)
// окно правки: у записи на правке — её окно; иначе выделение (нет — весь трек)
const target = computed(() => (editing.value ? { from: editing.value.from, to: editing.value.to } : applyWindow(props.sel)))
const targetCaption = computed(() => (target.value.to > 0 || target.value.from > 0
  ? t('engine.window', { from: target.value.from.toFixed(1), to: target.value.to > 0 ? target.value.to.toFixed(1) : t('studio.inserts.toEnd') })
  : t('engine.whole')))
const listenWin = computed(() => (editing.value && editing.value.to > editing.value.from
  ? { from: editing.value.from, to: Math.min(editing.value.to, editing.value.from + 15) }
  : previewWindow(props.sel, props.cursor, props.job.duration_sec)))

onMounted(loadAssets)
// гитарист по аккордам плана: подсказка, что треку нужен план
const gnotesHint = computed(() => (picked.value ? notesHint(picked.value.chain, t) : ''))
async function loadAssets() {
  try { assets.value = (await api.fxAssets()) || { amps: [], irs: [], kits: [] } } catch { /* старый воркер — ошибка покажется при «стало» */ }
}

// захват не выбран — по подсказке пресета (amp_hint), иначе первый загруженный: пресет с усилителем без захвата не считается
const withAmp = (c, hints) => fillAmp(c, assets.value.amps, hints)

function pickPreset(p) {
  presetId.value = p.id
  chain.value = withAmp(fromWorkerChain(p.chain, BLOCKS), p.amp_hint)
}
// выбор инструмента для раскрытой дорожки: готовые и свои — семейство, подвид, затем инструмент (поиск внутри
// подвида); цепочки без дорожек (мастер-склейка «на весь трек») — не инструмент дорожки: в пульте их нет
const rowTree = computed(() => (open.value
  ? instrumentTree(presetsFor(open.value, instruments.value).filter((p) => (p.stems || []).length), locale.value,
    t('instr.group.other')) : []))
const familyOpts = computed(() => familyOptions(rowTree.value))
const famNode = computed(() => rowTree.value.find((f) => f.family === familyId.value))
const groupOpts = computed(() => subOptions(famNode.value))
// один вариант — список не нужен
const showFamily = computed(() => familyOpts.value.length > 1)
const showGroup = computed(() => groupOpts.value.length > 1)
const itemOpts = computed(() => itemOptions(famNode.value?.subs.find((x) => x.group === groupId.value), locale.value))
watch(familyId, () => {
  const node = famNode.value
  if (node && !node.subs.some((x) => x.group === groupId.value)) groupId.value = node.subs[0].group
})
const picked = computed(() => instruments.value.find((p) => p.id === presetId.value) || null)
function pickById(id) {
  const p = instruments.value.find((x) => x.id === id)
  if (p) pickPreset(p)
}

function toggleRow(stem) {
  msg.value = ''
  err.value = ''
  editing.value = null
  if (open.value === stem) { open.value = ''; return }
  open.value = stem
  fine.value = false
  // сразу ничего не выбрано: сначала — инструмент из списка
  presetId.value = ''
  chain.value = []
  const at = pathOf(rowTree.value, '')
  familyId.value = at.family
  groupId.value = at.group
}

// «изменить» из «Правок трека»: редактор дорожки записи с её цепочкой, кнопка — «заменить»
function editRecord(rec) {
  const stem = (rec.stems || [])[0]
  if (!stem || !rec.engine) return
  // дорожки записи у трека нет (трек разделён на 4 дорожки) — сказать, а не молчать
  if (!rows.value.some((r) => r.stem === stem)) { err.value = t('desk.noRow', { stem: stemName(stem) }); return }
  open.value = stem
  presetId.value = ''
  msg.value = ''
  err.value = ''
  editing.value = { childId: rec.childId, from: rec.from || 0, to: rec.to || 0, label: rec.label || '' }
  chain.value = withAmp(fromWorkerChain(rec.engine, BLOCKS))
  fine.value = true               // правка записи — сразу её блоки
}
defineExpose({ editRecord })

function playStem(row) {
  const file = fileOf(row.stem)
  toggleArtifact(`s${props.job.id}:${file}`, `${stemName(row.stem)} · #${props.job.id}`,
    () => api.playFile(props.job.id, file, props.job.duration_sec))
}
const stemKey = (row) => `s${props.job.id}:${fileOf(row.stem)}`

async function makeStems() {
  busy.value = 'stems'
  err.value = ''
  try { await api.makeStems(props.job.id); emit('stems') } catch (e) { err.value = String(e) } finally { busy.value = '' }
}

// дорожки при открытии студии (настройка «Дорожки трека»); старый воркер без настройки — как «при открытии»
const stemsCfg = ref(null)   // {mode, model} — что делать и чем (RoFormer дольше)
const autoTried = ref(0)     // трек, для которого разделение уже запускали: сбой не повторяется по кругу
onMounted(async () => {
  try {
    const c = await api.workerConfig()
    stemsCfg.value = { mode: c.stems_auto || 'studio', model: c.stems_model || '' }
  } catch { stemsCfg.value = { mode: 'studio', model: '' } }   // воркер недоступен — разделение само упадёт с причиной
})
watch(() => [props.job.id, props.stemsLoaded, props.stems.length, stemsCfg.value], () => {
  if (!stemsCfg.value || busy.value || autoTried.value === props.job.id) return
  if (!shouldAutoSplit(props.stemsLoaded, props.stems, stemsCfg.value.mode)) return
  autoTried.value = props.job.id
  makeStems()
}, { immediate: true })
const splitTime = computed(() => t(stemsCfg.value && stemsCfg.value.model === 'bs-roformer-sw' ? 'desk.split.slow' : 'desk.split.fast'))

// место в стерео и «убрать» у строки дорожки — как у всех блоков: поменяли → «▶ стало / ▶ было» → «Применить»
// (в «Изменения трека»); до «Применить» правка висит у строки и в трек не идёт
const pend = ref({})   // дорожка → {place?, remove?}
const pendOf = (stem) => pend.value[stem] || {}
const hasPend = (stem) => !!(pendOf(stem).place || pendOf(stem).remove)
function setPend(stem, patch) {
  const cur = { ...pendOf(stem), ...patch }
  if (!cur.place) delete cur.place
  if (!cur.remove) delete cur.remove
  pend.value = { ...pend.value, [stem]: cur }
}
watch(() => props.job.id, () => { pend.value = {} })
async function previewRow(stem) {
  err.value = ''
  const jobId = props.job.id
  const w = listenWin.value
  const p = pendOf(stem)
  const sel = props.sel
  busy.value = 'row'
  try {
    const r = await inserts.previewWith(jobId, (list) => withPending(list, stem, p, sel))
    if (!r || !r.variant) throw new Error(t('studio.edits.none'))
    await rowAfter.play({ jobId, file: r.variant.file, dur: props.job.duration_sec, from: w.from, to: w.to })
  } catch (e) { err.value = String(e) } finally { busy.value = '' }
}
// «было» — трек со всеми правками без этой (нет правок — оригинал)
function playRowBefore() {
  const w = listenWin.value
  rowBefore.play({ jobId: props.job.id, file: inserts.mixFile(props.job.id) || props.job.audio_file,
    dur: props.job.duration_sec, from: w.from, to: w.to })
}
async function applyRow(stem) {
  err.value = ''
  msg.value = ''
  const jobId = props.job.id
  const p = pendOf(stem)
  const sel = props.sel
  busy.value = 'row'
  try {
    if (p.place) await inserts.setStemPlace(jobId, stem, p.place)
    if (p.remove) await inserts.addMutes(jobId, [removeRecord(stem, sel)])
    setPend(stem, { place: null, remove: false })
    msg.value = t('desk.row.applied')
    emit('applied')
  } catch (e) { err.value = String(e) } finally { busy.value = '' }
}

// «стало»: превью воркера на окне прослушивания (повтор тех же настроек — тот же файл)
async function playAfter() {
  err.value = ''
  msg.value = ''
  const jobId = props.job.id     // всё — до первого await: дорожку и цепочку могут сменить
  const req = { source: open.value, chain: toWorkerChain(chain.value), output: solo.value ? 'solo' : 'mix', preview: true, ...listenWin.value }
  await toggleArtifact('desk-after', t('instr.after'), async () => {
    try {
      await ensureKits(api, req.chain, assets.value.kits, () => { busy.value = 'kit' })
      busy.value = 'preview'
      const r = await api.applyFx(jobId, req)
      if (r && r.clipped) msg.value = t('instr.clipped')
      await api.playFile(jobId, r.file, r.duration_sec || 15)
    } catch (e) { err.value = String(e); throw e } finally { busy.value = ''; loadAssets() }
  })
}

// «было»: тот же кусок без обработки — дорожка соло или весь микс
function playBeforeWin() {
  const w = listenWin.value
  before.play({ jobId: props.job.id, file: solo.value ? fileOf(open.value) : props.job.audio_file,
    dur: props.job.duration_sec, from: w.from, to: w.to })
}

// «в трек» / «заменить»: проверка коротким превью → запись реестра → пересборка
async function apply() {
  err.value = ''
  msg.value = ''
  const p = instruments.value.find((x) => x.id === presetId.value)
  const ed = editing.value
  const snap = {
    jobId: props.job.id, dur: props.job.duration_sec, src: open.value, ...target.value,
    chain: toWorkerChain(chain.value),
    label: p ? t('engine.label', { name: tr(p.name) }) : (ed && ed.label) || t('engine.label', { name: t('desk.own') }),
  }
  busy.value = 'apply'
  try {
    const rec = await applyEngine(api, snap, { oldMsg: t('instr.engine.old'), onKit: () => { busy.value = 'kit' } })
    if (ed) await inserts.replaceEngine(snap.jobId, ed.childId, { chain: rec.chain, label: rec.label })
    else await inserts.addStemEngine(snap.jobId, rec)
    // пока шла запись, могли открыть на правку другую запись — её не сбрасывать
    if (editing.value === ed) editing.value = null
    msg.value = t('engine.applied')
    emit('applied')
  } catch (e) { err.value = String(e) } finally { busy.value = ''; loadAssets() }
}

// ритм-секция набором: все части барабанов и бас, что есть у трека, — одной пересборкой
// набор (живой или драм-машина) и обработка (по умолчанию — комната)
const drumKit = ref('osdk')
const treatment = ref('room')
const kitOptions = computed(() => DRUM_KITS.map((k) => ({ value: k.id, label: tr(k.name) })))
const treatOptions = computed(() => DRUM_TREATMENTS.map((x) => ({ value: x.id, label: tr(x.name) })))
const kitNote = computed(() => tr((DRUM_KITS.find((k) => k.id === drumKit.value) || {}).note))
const rhythm = computed(() => rhythmSection(props.stems.map((s) => s.name), fxPresets, treatment.value, locale.value, drumKit.value))
// окно ритм-секции — всегда выделение (нет — весь трек), не окно открытой на правку записи
const rhythmWin = computed(() => applyWindow(props.sel))
const rhythmCaption = computed(() => (rhythmWin.value.to > 0
  ? t('engine.window', { from: rhythmWin.value.from.toFixed(1), to: rhythmWin.value.to.toFixed(1) })
  : t('engine.whole')))
// «▶ прослушать» ритм-секции: трек с наборами на кусок (выделение или 15 с от курсора), в список не пишется
async function previewRhythm() {
  err.value = ''
  msg.value = ''
  const jobId = props.job.id
  const dur = props.job.duration_sec
  const w = listenWin.value
  const items = rhythm.value.map((r) => ({ ...r, ...rhythmWin.value }))
  busy.value = 'rhythm'
  try {
    const recs = []
    for (const it of items) {
      recs.push(await applyEngine(api, { jobId, dur, src: it.stem, from: it.from, to: it.to, chain: it.chain, label: it.label },
        { oldMsg: t('instr.engine.old'), onKit: () => { busy.value = 'kit' } }))
      busy.value = 'rhythm'
    }
    const extra = recs.map((r) => ({ instId: 'engine', from: r.from, to: r.to, db: 0, stems: [r.stem], engine: r.chain }))
    const res = await inserts.previewWith(jobId, (list) => [...list, ...extra])
    if (!res || !res.variant) throw new Error(t('studio.edits.none'))
    await rhythmAfter.play({ jobId, file: res.variant.file, dur, from: w.from, to: w.to })
  } catch (e) { err.value = String(e) } finally { busy.value = ''; loadAssets() }
}

async function applyRhythm() {
  err.value = ''
  msg.value = ''
  const jobId = props.job.id
  const dur = props.job.duration_sec
  const items = rhythm.value.map((r) => ({ ...r, ...rhythmWin.value }))   // всё — до первого await
  busy.value = 'rhythm'
  try {
    // каждая цепочка — проверка коротким превью, как у «в трек»: запись, которую воркер не примет
    // (движок выключен, нет набора, старый воркер), роняла бы каждую следующую пересборку трека
    const recs = []
    for (const it of items) {
      recs.push(await applyEngine(api, { jobId, dur, src: it.stem, from: it.from, to: it.to, chain: it.chain, label: it.label },
        { oldMsg: t('instr.engine.old'), onKit: () => { busy.value = 'kit' } }))
      busy.value = 'rhythm'
    }
    await inserts.addStemEngines(jobId, recs)
    msg.value = t('desk.rhythm.done', { n: items.length })
    emit('applied')
  } catch (e) { err.value = String(e) } finally { busy.value = ''; loadAssets() }
}

// громкость дорожки к треку (замер воркера): почти неслышную двигать и обрабатывать бесполезно (#689: райд −35 дБ)
const levels = computed(() => stemLevels(props.stems))
const levelText = (stem) => (stem in levels.value ? `${levels.value[stem] > 0 ? '+' : ''}${Math.round(levels.value[stem])} ${t('studio.inserts.dbUnit')}` : '')
const levelMark = (stem) => (stem in levels.value ? stemAudibility(levels.value[stem]) : '')

// место дорожки в стерео: отпустил ползунок — запись «место» заменяется, трек пересобирается
const placeOf = (stem) => stemPlace(inserts.appliedFor(props.job.id), stem)

async function installKit(name) {
  err.value = ''
  busy.value = 'kit'
  try { await installKitOn(api, name); await loadAssets() } catch (e) { err.value = String(e) } finally { busy.value = '' }
}
async function upload(kind) {
  err.value = ''
  try {
    if (await api.uploadFxAsset(kind)) { await loadAssets(); chain.value = withAmp(chain.value, instruments.value.find((x) => x.id === presetId.value)?.amp_hint) }
  } catch (e) { err.value = String(e) }
}
</script>

<template>
  <!-- дорожек нет: идёт разделение — что происходит и сколько ждать; иначе — кнопка -->
  <p v-if="!stems.length && busy === 'stems'" class="desk-split"><span class="pulse">⟳</span>
    {{ t('desk.split.busy', { time: splitTime }) }}</p>
  <div v-else-if="!stems.length" class="dsp-row">
    <span class="muted">{{ t('desk.noStems') }}</span>
    <button class="primary small" :disabled="!!busy" @click="makeStems">{{ t('instr.stems.make') }}</button>
  </div>
  <div v-if="noParts" class="dsp-row">
    <span class="muted">{{ t('desk.noParts') }}</span>
    <button class="ghost small-btn" :disabled="!!busy" :title="t('desk.resplit.tip')" @click="makeStems">
      {{ busy === 'stems' ? t('instr.stems.busy') : t('desk.resplit') }}</button>
  </div>
  <div v-if="stems.length" class="dsp-row">
    <span class="muted">{{ t('desk.rhythm') }}:</span>
    <span :title="kitNote"><VSelect v-model="drumKit" :options="kitOptions" style="max-width: 260px" /></span>
    <span :title="t('desk.room.tip')"><VSelect v-model="treatment" :options="treatOptions" style="max-width: 190px" /></span>
    <span v-if="!rhythm.length" class="muted">{{ t('desk.rhythm.none') }}</span>
    <template v-else>
      <span class="muted">{{ rhythm.map((r) => stemName(r.stem)).join(', ') }} · {{ rhythmCaption }}</span>
      <button class="ghost small-btn" :disabled="!!busy" :title="t('desk.rhythm.listen.tip')" @click="previewRhythm">
        <template v-if="busy === 'rhythm' || busy === 'kit'">{{ busy === 'kit' ? t('instr.kit.busy') : t('instr.busy') }}</template>
        <template v-else>{{ playBtn('desk-rhythm-after') }} {{ t('desk.rhythm.listen') }}</template></button>
      <button class="primary small" :disabled="!!busy" :title="t('desk.rhythm.tip')" @click="applyRhythm">{{ t('desk.apply') }}</button>
    </template>
  </div>
  <p v-if="msg && !open" class="muted">{{ msg }}</p>
  <div v-for="row in rows" :key="row.stem" class="desk-row" :class="{ open: open === row.stem }">
    <div class="desk-head">
      <button class="ghost play-mini" :class="{ stop: isPlaying(stemKey(row)) }" :disabled="playBusy[stemKey(row)]"
              :title="t('desk.solo.tip')" @click="playStem(row)">{{ playBtn(stemKey(row)) }}</button>
      <button class="ghost desk-name desk-select" :class="{ on: props.active === row.stem }"
              :title="(t('desk.what.' + row.stem) || '') + ' · ' + t('desk.select.tip')"
              @click="emit('select', props.active === row.stem ? '' : row.stem)">{{ stemName(row.stem) }}</button>
      <span v-if="levelText(row.stem)" class="muted desk-level" :class="levelMark(row.stem)" :title="t('desk.level.tip')">
        {{ levelText(row.stem) }}<template v-if="levelMark(row.stem)"> · {{ t('desk.level.' + levelMark(row.stem)) }}</template></span>
      <span v-for="e in row.edits" :key="e.childId" class="desk-chip" :class="{ off: e.off }"
            :title="e.off ? t('desk.chip.off') : ''">{{ insertTitle(e, names) }} · {{ insertWindow(e, win) }}</span>
      <span class="spacer"></span>
      <button class="toggle small-btn" :class="{ on: pendOf(row.stem).remove }" :disabled="!!busy" :title="t('desk.remove.tip')"
              @click="setPend(row.stem, { remove: !pendOf(row.stem).remove })">{{ t('desk.remove') }}</button>
      <PlaceControls :model-value="pendOf(row.stem).place || placeOf(row.stem)" :disabled="!!busy"
                     @update:model-value="(p) => setPend(row.stem, { place: p })" />
      <button class="ghost small-btn" :class="{ on: open === row.stem }" @click="toggleRow(row.stem)">
        {{ open === row.stem ? t('desk.close') : t('desk.sound') }}</button>
    </div>
    <div v-if="hasPend(row.stem)" class="dsp-row desk-pend">
      <span class="muted">{{ t('desk.row.pending') }}</span>
      <button class="ghost small-btn" :disabled="!!busy" :title="t('instr.after.tip')" @click="previewRow(row.stem)">
        <template v-if="busy === 'row'">{{ t('instr.busy') }}</template><template v-else>{{ playBtn('desk-row-after') }} {{ t('instr.after') }}</template></button>
      <button class="ghost small-btn" :title="t('instr.before.tip')" @click="playRowBefore">{{ playBtn('desk-row-before') }} {{ t('instr.before') }}</button>
      <span class="muted">{{ t('desk.listen', { from: listenWin.from.toFixed(1), to: listenWin.to.toFixed(1) }) }}</span>
      <span class="spacer"></span>
      <button class="primary small" :disabled="!!busy" :title="t('desk.row.apply.tip')" @click="applyRow(row.stem)">{{ t('desk.apply') }}</button>
      <button class="ghost small-btn" :disabled="!!busy" @click="setPend(row.stem, { place: null, remove: false })">{{ t('common.cancel') }}</button>
    </div>
    <div v-if="open === row.stem" class="desk-editor">
      <p v-if="editing" class="muted">{{ t('desk.editing') }}</p>
      <div class="desk-pick">
        <label v-if="showFamily">
          <span class="muted">{{ t('instr.family') }}</span>
          <VSelect v-model="familyId" :options="familyOpts" />
        </label>
        <label v-if="showGroup">
          <span class="muted">{{ t('instr.group') }}</span>
          <VSelect v-model="groupId" :options="groupOpts" />
        </label>
        <label>
          <span class="muted">{{ t('instr.item') }}</span>
          <VSelect :model-value="presetId" :options="itemOpts" searchable :placeholder="t('desk.pick')" @update:model-value="pickById" />
        </label>
        <button class="ghost small-btn" :class="{ on: fine }" :disabled="!chain.length" :title="t('desk.fine.tip')"
                @click="fine = !fine">{{ fine ? t('desk.fine.hide') : t('desk.fine') }}</button>
      </div>
      <p v-if="picked" class="muted voice-hint">{{ tr(picked.note) }}</p>
      <p v-if="gnotesHint" class="muted voice-hint">{{ gnotesHint }}</p>
      <ChainEditor v-if="fine" v-model="chain" :assets="assets" :busy="busy" @upload="upload" @install-kit="installKit" />
      <p v-if="missing.length" class="err">{{ t('instr.needAmp') }}</p>
      <div class="dsp-row">
        <label class="muted" :title="t('instr.solo.tip')"><input v-model="solo" type="checkbox" /> {{ t('instr.solo') }}</label>
        <button class="ghost small-btn" :disabled="!ready" :title="t('instr.after.tip')" @click="playAfter">
          <template v-if="busy === 'preview' || busy === 'kit'">{{ busy === 'kit' ? t('instr.kit.busy') : t('instr.busy') }}</template>
          <template v-else>{{ playBtn('desk-after') }} {{ t('instr.after') }}</template></button>
        <button class="ghost small-btn" :title="t('instr.before.tip')" @click="playBeforeWin">{{ playBtn('desk-before') }} {{ t('instr.before') }}</button>
        <span class="muted">{{ t('desk.listen', { from: listenWin.from.toFixed(1), to: listenWin.to.toFixed(1) }) }}</span>
        <span class="spacer"></span>
        <span class="muted">{{ targetCaption }}</span>
        <button class="primary small" :disabled="!ready" :title="t('engine.apply.tip')" @click="apply">
          {{ busy === 'apply' ? t('instr.busy') : editing ? t('desk.replace') : t('desk.apply') }}</button>
        <button v-if="editing" class="ghost small-btn" @click="editing = null">{{ t('common.cancel') }}</button>
      </div>
      <p v-if="msg" class="muted">{{ msg }}</p>
    </div>
  </div>
  <p v-if="err" class="err">{{ err }}</p>
</template>

<style scoped>
.desk-split { font-size: 13px; margin: 4px 0; }
.desk-pend { margin: 2px 0 4px 28px; }
.desk-row { border-bottom: 1px solid var(--line, #2a2a35); padding: 4px 0; }
.desk-row.open { background: var(--panel-2, rgba(255, 255, 255, 0.02)); }
.desk-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.desk-select { font-weight: 600; min-width: 90px; text-align: left; }
.desk-select.on { border-color: var(--accent, #7aa2f7); color: var(--accent, #7aa2f7); }
.desk-level { font-size: 12px; }
.desk-level.silent { opacity: 0.6; font-style: italic; }
.desk-chip { font-size: 12px; border: 1px solid var(--line, #2a2a35); border-radius: 10px; padding: 1px 8px; }
.desk-chip.off { opacity: 0.45; text-decoration: line-through; }
.desk-editor { padding: 6px 0 6px 34px; }
.spacer { flex: 1; }
.small-btn.on { border-color: var(--accent, #7aa2f7); }
.desk-pick { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px; margin: 6px 0; }
.desk-pick label { display: flex; flex-direction: column; gap: 4px; min-width: 220px; }
</style>
