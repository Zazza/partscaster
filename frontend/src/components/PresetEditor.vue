<script setup>
// Редактор пресета звука (модалка из библиотеки). Встроенный пресет открывается
// как копия (сохранение создаёт свой); recipe-поля без правок (parts) пробрасываются как есть.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import VSelect from '../VSelect.vue'
import ChainEditor from './ChainEditor.vue'
import PlaceControls from './PlaceControls.vue'
import PresetCatalog from './PresetCatalog.vue'
import { usePlayer } from '../composables/usePlayer.js'
import { useWindowPlay } from '../composables/useWindowPlay.js'
import { usePresetDemo } from '../composables/usePresetDemo.js'
import { watchQueueItem } from '../composables/useProcQueue.js'
import { presetMasterChain, editorSnapshot } from '../presetEdit.js'
import { useConfirm } from '../composables/useConfirm.js'
import BLOCKS from '../fxBlocks.json'
import { fxPresets } from '../fxPresets.js'
import { fromWorkerChain, missingRequired, toWorkerChain } from '../fxChain.js'
import { ensureKits } from '../engineRun.js'
import { previewWindow } from '../trackDesk.js'
import { presetOptions, PRESET_FAMILIES } from '../soundPresets.js'
import {
  DRUM_PARTS, bodyFromModel, editorCatalog, masterPresets, newModel, previewSource,
  rhythmSpecs, specFromItem, specKind, specKindOk, toModel, toggleStem,
} from '../presetEdit.js'

const props = defineProps({
  preset: { type: Object, default: null },     // null — новый пресет
  presets: { type: Array, default: () => [] }, // для «финал как у…»
})
const emit = defineEmits(['close', 'saved'])
const { t, locale } = useI18n()
const { toggleArtifact, playBtn } = usePlayer()
const demo = usePresetDemo()

const isCopy = ref(!props.preset || !!props.preset.builtin)
const model = ref(props.preset ? toModel(props.preset) : newModel())
if (props.preset && props.preset.builtin) {
  // пометка копии с умной обрезкой: суффикс «(копия)» не должен отрезаться
  const suf = ' ' + t('preset.edit.copy')
  const n = model.value.name
  model.value.name = n.length + suf.length <= 80 ? n + suf : n.slice(0, 80 - suf.length) + suf
}

const busy = ref('')               // превью, скачивание набора
const action = ref('')             // 'save' | 'apply' — не сбрасывается концом превью
const applyLive = ref(false)       // применение идёт в очереди обработки: ход — в msg
let closed = false                 // окно закрыто — ждущее демо не применяет пресет молча
let applyUnwatch = null            // слежение за записью очереди применения
const msg = ref('')
const err = ref('')

// эталон прослушивания: демо жанра (по семье пресета) или свой трек
const refFamily = ref((props.preset && props.preset.family) || PRESET_FAMILIES[0])
const ownJobId = ref(0)
const ownJobs = ref([])
const ownStems = ref(null)        // null — дорожки ещё не спрашивали
const ownStemsBusy = ref(false)
// сменили свой эталон — его дорожки спросим заново
watch(ownJobId, () => { ownStems.value = null })
const demoState = computed(() => demo.demos.value[refFamily.value] || {})
const ownJob = computed(() => ownJobs.value.find((j) => j.id === ownJobId.value) || null)
const ownOptions = computed(() => ownJobs.value.map((j) => ({
  value: j.id, label: `#${j.id} · ${j.title}`, title: j.title, search: j.style || '',
})))
const refDuration = computed(() => (ownJobId.value ? (ownJob.value?.duration_sec || 0) : (demoState.value.durationSec || 0)))
// окно прослушивания: с 20-й секунды, 15 с — как превью эффектов без выделения
const listenWin = computed(() => previewWindow(null, 20, refDuration.value || 0))

const fxOn = ref(true)
const assets = ref({ amps: [], irs: [], kits: [] })

let poll = null
onMounted(async () => {
  poll = setInterval(() => { demo.refresh() }, 5000)
  try { assets.value = (await api.fxAssets()) || assets.value } catch { /* старый воркер — ошибка покажется при прослушивании */ }
  try { fxOn.value = !!((await api.workerConfig()) || {}).fx_preview } catch { /* предположим, что поддерживается */ }
  try {
    const all = (await api.jobs()) || []
    ownJobs.value = all.filter((j) => j.status === 'done' && !j.draft
      && (j.folder || '').trim() !== 'Демо' && !j.parent_id && !j.overdub_of)
  } catch { /* списка нет — останется демо */}
  // эталон пресета — свой трек, если он ещё существует
  const refId = (props.preset && props.preset.reference_job_id) || 0
  if (refId && ownJobs.value.some((j) => j.id === refId)) ownJobId.value = refId
  // демо не готовим заранее: генерация — при первом прослушивании (открыл и закрыл — видеокарта не занята)
})
onBeforeUnmount(() => { if (poll) clearInterval(poll) })

async function loadAssets() {
  try { assets.value = (await api.fxAssets()) || assets.value } catch { /* как было */ }
}

// какой эталон выбран сейчас: ответ ensureReference про сменённый эталон не играется
const refKey = () => (ownJobId.value ? `own:${ownJobId.value}` : `demo:${refFamily.value}`)

/** Эталон готов к прослушиванию: демо — ensure + дорожки, свой трек — дорожки лениво. */
async function ensureReference() {
  if (ownJobId.value) {
    const j = ownJob.value
    if (!j) throw new Error(t('preset.edit.noStems'))
    if (ownStems.value == null && !ownStemsBusy.value) {
      ownStemsBusy.value = true
      err.value = t('preset.edit.demo.stems')
      try {
        let names = ((await api.jobStems(j.id)) || []).map((s) => s.name)
        if (!names.length) {
          await api.makeStems(j.id)
          names = ((await api.jobStems(j.id)) || []).map((s) => s.name)
        }
        if (ownJobId.value === j.id) ownStems.value = names   // эталон сменили, пока ждали, — ответ не про него
      } catch (e) {
        if (ownJobId.value === j.id) ownStems.value = null     // сбой разделения — следующее ▶ повторит, а не «нет дорожки»
        throw e
      } finally { ownStemsBusy.value = false; err.value = '' }
    }
    return { jobId: j.id, file: j.audio_file, available: new Set(ownStems.value || []), dur: j.duration_sec || 0 }
  }
  await demo.ensure(refFamily.value)
  await demo.ensureStems(refFamily.value)
  const st = demo.demos.value[refFamily.value] || {}
  return { jobId: st.jobId, file: st.file || '', available: demo.available(refFamily.value), dur: st.durationSec || 0 }
}

const refBusy = computed(() => ownStemsBusy.value
  || (!ownJobId.value && ['cooking', 'stems'].includes(demoState.value.status)))

// прослушивание: «стало» — превью движка на дорожке эталона, «было» — тот же кусок без обработки
const solo = ref(false)
const before = useWindowPlay('pe-before', () => t('instr.before'))

async function playAfter(chain, stems, key, label) {
  msg.value = ''
  err.value = ''
  if (!fxOn.value) { err.value = t('instr.engine.old'); return }
  let reference
  const want = refKey()
  try { reference = await ensureReference() } catch (e) { err.value = String(e); return }   // сбой подготовки — на экран
  if (refKey() !== want) return          // эталон сменили, пока готовили, — не играть старый
  const src = previewSource(stems, reference.available)
  if (src.blocked === 'roformer') { err.value = t('preset.edit.needRoformer'); return }
  if (!src.source) { err.value = t('preset.edit.noStems'); return }
  const jobId = reference.jobId
  const w = listenWin.value
  const req = { source: src.source, chain, output: solo.value ? 'solo' : 'mix', preview: true, from: w.from, to: w.to }
  await toggleArtifact(key, label, async () => {
    try {
      await ensureKits(api, req.chain, assets.value.kits, () => { busy.value = 'kit' })
      busy.value = 'preview'
      const r = await api.applyFx(jobId, req)
      if (r && r.clipped) msg.value = t('instr.clipped')
      await api.playFile(jobId, r.file, r.duration_sec || 15)
    } catch (e) { err.value = String(e); throw e } finally { busy.value = ''; loadAssets() }
  })
}

function playBefore() {
  const want = refKey()
  ensureReference().then((reference) => {
    if (refKey() !== want || !reference.jobId) return   // эталон сменили, пока готовили, — не играть старый
    // демо ещё готовится — «было» пока нечего играть
    if (!reference.file || !reference.dur) { err.value = t('preset.edit.demo.cooking'); return }
    const w = listenWin.value
    before.play({ jobId: reference.jobId, file: reference.file, dur: reference.dur, from: w.from, to: w.to })
  }).catch((e) => { err.value = String(e) })
}

const target = ref('bass')
const catalog = computed(() => editorCatalog(target.value, fxPresets, locale.value))

function previewItem(p) {
  const sp = specFromItem(p, target.value, assets.value.amps)
  playAfter(sp.engine, sp.stems, 'pc:' + p.id, tr(p.name))
}
function addItem(p) {
  msg.value = ''
  model.value.specs.push(specFromItem(p, target.value, assets.value.amps))
}
async function addRhythm({ kit, treat }) {
  msg.value = ''
  err.value = ''
  try {
    const reference = await ensureReference()
    const specs = rhythmSpecs(reference.available, treat, kit, locale.value)
    if (!specs.length) { err.value = t('preset.edit.noStems'); return }
    model.value.specs.push(...specs)
  } catch (e) { err.value = String(e) }
}

const tr = (lbl) => (lbl && (lbl[locale.value] || lbl.ru)) || ''
const stemName = (s) => t('studio.dsp.target.' + s)
// все дорожки чипов записи: основные + гитара/клавиши + части барабанов
const STEM_CHOICES = ['vocals', 'drums', 'bass', 'guitar', 'piano', 'other', ...DRUM_PARTS]

// подпись записи: дорожки и что с ними — как в библиотеке
function specLine(sp) {
  const what = sp.engine ? (sp.edit ? sp.edit.filter((b) => b.on).map((b) => b.type).join(' → ')
      : sp.engine.map((b) => b.type).join(' → '))
    : sp.steps ? sp.steps.filter((x) => !x.off).map((x) => x.chain).join(' → ') : sp.chain
  return `${(sp.stems || []).map(stemName).join(', ')}: ${what}`
}
// тонкая правка цепочки engine-записи: edit-форма (fxChain), при сохранении — обратно в worker-форму
function toggleEdit(sp, open) {
  if (!open) return
  // раскрыть форму — не правка: снимок сравнивает цепочки в нормальной форме (editorSnapshot)
  if (!sp.edit) sp.edit = fromWorkerChain(sp.engine || [], BLOCKS)
}
function syncEdit() {
  for (const sp of model.value.specs) {
    if (sp.edit && specKind(sp) !== 'steps' && specKind(sp) !== 'chain') sp.engine = toWorkerChain(sp.edit)
  }
}

// записи с видом ffmpeg на частях барабанов применение не примет — не даём сохранить молча
const kindIssues = computed(() => model.value.specs
  .filter((sp) => !specKindOk(sp.stems, specKind(sp))).map((sp) => specLine(sp)))

// мастер: готовые чипы + тонкая правка + цель громкости (target_lufs пресета)
const MASTERS = masterPresets(fxPresets)
const masterPicked = ref('')
const masterEdit = ref(props.preset && (props.preset.master || []).length ? fromWorkerChain(props.preset.master, BLOCKS) : [])
function pickMaster(m) {
  masterPicked.value = m.id
  masterEdit.value = fromWorkerChain(m.chain, BLOCKS)
}
const masterMissing = computed(() => missingRequired(masterEdit.value, BLOCKS))
function masterWorkerChain() {
  // превью — ровно то, что применит Go (presetMaster): без цели ограничитель остаётся как в цепочке
  return presetMasterChain(toWorkerChain(masterEdit.value), model.value.targetLufs)
}
// мастер — на весь микс: источник mix, дорожки не нужны
function playMasterAfter() {
  msg.value = ''
  err.value = ''
  if (!fxOn.value) { err.value = t('instr.engine.old'); return }
  if (!masterEdit.value.length && model.value.targetLufs == null) return   // нечего слушать: ни цепочки, ни цели
  const want = refKey()
  ensureReference().then((reference) => {
    if (refKey() !== want) return
    const jobId = reference.jobId
    const w = listenWin.value
    const req = { source: 'mix', chain: masterWorkerChain(), output: 'mix', preview: true, from: w.from, to: w.to }
    return toggleArtifact('pe-master', t('instr.after'), async () => {
      try {
        await ensureKits(api, req.chain, assets.value.kits, () => { busy.value = 'kit' })
        busy.value = 'preview'
        const r = await api.applyFx(jobId, req)
        if (r && r.clipped) msg.value = t('instr.clipped')
        await api.playFile(jobId, r.file, r.duration_sec || 15)
      } catch (e) { err.value = String(e); throw e } finally { busy.value = ''; loadAssets() }
    })
  }).catch((e) => { err.value = String(e) })
}

async function uploadAsset(kind) {
  try { await api.uploadFxAsset(kind) } catch (e) { err.value = String(e) } finally { loadAssets() }
}
async function installKit(name) {
  busy.value = 'kit'
  try { await api.installFxKit(name) } catch (e) { err.value = String(e) } finally { busy.value = ''; loadAssets() }
}

// финал: «как у другого пресета» — копирует цепочку и цель громкости, как в студии
const finalFrom = ref(0)
const finalOptions = computed(() => [{ value: 0, label: t('preset.final.none') },
  ...presetOptions(props.presets.filter((p) => (p.final || []).length || p.target_lufs != null))
    .map((o) => (typeof o.value === 'number' ? { ...o, label: t('preset.final.of', { name: o.label }) } : o))])
const finalLine = computed(() => (model.value.final || []).filter((x) => !x.off).map((x) => x.chain).join(' → '))
function pickFinal(id) {
  const donor = props.presets.find((p) => p.id === id) || null
  model.value.final = JSON.parse(JSON.stringify((donor && donor.final) || []))
  model.value.targetLufs = donor && donor.target_lufs != null ? donor.target_lufs : null
}

const canSave = computed(() => !busy.value && !action.value && model.value.name.trim().length > 0 && !kindIssues.value.length)
async function save() {
  if (action.value) return
  action.value = 'save'
  msg.value = ''
  err.value = ''
  try {
    syncEdit()
    model.value.master = masterEdit.value.length ? toWorkerChain(masterEdit.value) : []
    const body = bodyFromModel(model.value)
    const sentSnap = snapshot()          // что ушло на сервер: правки во время ответа останутся «несохранёнными»
    const saved = isCopy.value ? await api.soundPresetCreate(body) : await api.soundPresetUpdate(model.value.id, body)
    // копия/новый сохранились — дальше это свой пресет, обновляем, а не плодим дубликаты
    model.value.id = saved.id
    isCopy.value = false
    msg.value = t('preset.saved', { name: saved.name })
    savedSnap = sentSnap
    emit('saved', saved)
  } catch (e) { err.value = String(e) } finally { action.value = '' }
}

async function applyToRef() {
  if (action.value || applyLive.value) return
  action.value = 'apply'
  msg.value = ''
  err.value = ''
  if (applyUnwatch) { applyUnwatch(); applyUnwatch = null }
  try {
    syncEdit()
    model.value.master = masterEdit.value.length ? toWorkerChain(masterEdit.value) : []
    let jobId = 0
    if (ownJobId.value) {
      jobId = ownJob.value?.id || 0
    } else {
      // демо должно доготовиться — применять к недоделанному бессмысленно; семья — та, что была при нажатии
      const fam = refFamily.value
      if ((demo.demos.value[fam] || {}).status !== 'ready') {
        msg.value = t('preset.edit.demo.cooking')
      }
      await demo.ensure(fam)
      const st = await demo.waitReady(fam, 600, () => closed)
      if (closed) return                 // окно закрыли, пока демо доготавливалось — не применять
      if (st !== 'ready') throw new Error(t('preset.edit.demo.err', { err: (demo.demos.value[fam] || {}).err || st }))
      jobId = demo.demos.value[fam]?.jobId || 0
    }
    if (!jobId) throw new Error(t('preset.edit.noStems'))
    // применение идёт в очереди обработки (минуты): ход — в сообщении, редактор жив
    msg.value = t('preset.applying', { name: model.value.name })
    const qid = await api.applySoundPresetWith(jobId, bodyFromModel(model.value))
    action.value = ''                    // дальше фон: можно править и сохранять дальше
    applyLive.value = true
    const it = await new Promise((resolve) => {
      applyUnwatch = watchQueueItem(qid, (item) => {
        if (closed) { if (applyUnwatch) applyUnwatch(); applyUnwatch = null; resolve(null); return }
        if (item.state === 'waiting') { msg.value = t('preset.apply.wait', { name: model.value.name }); return }
        if (item.state === 'running') { msg.value = t('preset.applying', { name: model.value.name }); return }
        resolve(item)
      })
    })
    applyUnwatch = null
    if (!it) return                      // окно закрыли, пока применялось
    if (it.state === 'error') throw new Error(it.err || t('preset.apply.err', { name: model.value.name }))
    if (it.state === 'canceled') { msg.value = t('preset.apply.canceled'); return }
    // версия демо спрятана в папке «Демо» — играем её сразу, искать в списке незачем
    if (!ownJobId.value) {
      const v = ((await api.jobs()) || []).find((j) => j.id === it.result_job_id)
      if (v) {
        msg.value = t('preset.edit.applied.play', { id: it.result_job_id })
        await toggleArtifact(`demo${v.id}`, `#${v.id} ${v.title}`, () => api.playAudio(v.id),
          { jobId: v.id, file: v.audio_file, dur: v.duration_sec })
        return
      }
    }
    msg.value = t('preset.edit.applied', { id: it.result_job_id })
  } catch (e) { msg.value = ''; err.value = String(e) } finally {
    applyLive.value = false
    action.value = ''
  }
}

// закрытие: несохранённые правки — подтверждение
const { askConfirm } = useConfirm()
// сравнение для «несохранённых правок»: без раскрытой формы цепочки (edit) — раскрыть и закрыть не значит изменить
// правка тонкой цепочки живёт в sp.edit (в engine — только при сохранении): снимок берёт её рабочую форму; id не
// в счёт (после сохранения нового пресета он появляется — это не правка)
const snapshot = () => editorSnapshot(model.value, masterEdit.value)
let savedSnap = ''
onMounted(() => { savedSnap = snapshot() })
function close() {
  if (snapshot() === savedSnap) { emit('close'); return }
  askConfirm(t('preset.edit.unsaved.title'), t('preset.edit.unsaved.body'), () => emit('close'))
}
onBeforeUnmount(() => { closed = true; if (applyUnwatch) applyUnwatch() })
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal preset-editor-modal">
      <div class="modal-head">
        <h2>{{ t('preset.edit.title') }} <span class="muted">{{ model.name || t('preset.edit.new') }}</span></h2>
        <span class="spacer"></span>
        <button class="ghost" :title="t('common.close')" @click="close"><AppIcon name="x" /></button>
      </div>

      <div class="dsp-row">
        <input v-model="model.name" maxlength="80" :placeholder="t('preset.save.name')" style="max-width: 260px" />
        <input v-model="model.note" maxlength="500" :placeholder="t('preset.save.note')" class="preset-note" />
      </div>

      <div class="dsp-row preset-ref">
        <span class="muted" :title="t('preset.edit.ref.tip')">{{ t('preset.edit.ref') }}</span>
        <template v-for="fam in PRESET_FAMILIES" :key="fam">
          <button v-if="!ownJobId" class="ghost small-btn" :class="{ on: refFamily === fam }"
                  :title="t('preset.edit.ref.demo', { family: fam })" @click="refFamily = fam">{{ fam }}</button>
        </template>
        <VSelect v-model="ownJobId" :options="[{ value: 0, label: '—' }, ...ownOptions]" searchable
                 :title="t('preset.edit.ref.tip')" style="max-width: 260px" />
        <label class="muted"><input v-model="solo" type="checkbox" /> {{ t('instr.solo') }}</label>
        <button class="ghost small-btn" @click="playBefore">{{ playBtn('pe-before') }} {{ t('instr.before') }}</button>
        <span class="muted">{{ t('preset.edit.window', { from: Math.floor(listenWin.from), to: Math.floor(listenWin.to) }) }}</span>
      </div>
      <p v-if="!ownJobId && demoState.status === 'cooking'" class="muted">{{ t('preset.edit.demo.cooking') }}</p>
      <p v-else-if="!ownJobId && demoState.status === 'stems'" class="muted">{{ t('preset.edit.demo.stems') }}</p>
      <p v-else-if="!ownJobId && demoState.status === 'error'" class="err">
        {{ t('preset.edit.demo.err', { err: demoState.err }) }}
        <button class="ghost small-btn" @click="demo.reset(refFamily); demo.ensure(refFamily)">{{ t('preset.edit.demo.again') }}</button>
      </p>

      <h3 class="muted">{{ t('preset.edit.tracks') }}</h3>
      <p v-if="!model.specs.length" class="muted">{{ t('preset.edit.empty') }}</p>
      <div v-for="sp in model.specs" :key="sp.uid" class="preset-spec">
        <div class="dsp-row">
          <span class="preset-spec-line">{{ specLine(sp) }}</span>
          <button v-if="specKind(sp) === 'engine'" class="ghost small-btn" :disabled="refBusy"
                  :title="t('instr.after')" @click="playAfter(sp.edit ? toWorkerChain(sp.edit) : sp.engine, sp.stems, 'pe:' + sp.uid, specLine(sp))">
            {{ playBtn('pe:' + sp.uid) }}</button>
          <button class="ghost small-btn" :title="t('preset.edit.spec.remove')" @click="model.specs = model.specs.filter((x) => x !== sp)"><AppIcon name="x" /></button>
        </div>
        <div class="dsp-row preset-spec-ctrl">
          <button v-for="s in STEM_CHOICES" :key="s" class="ghost small-btn stem-chip" :class="{ on: sp.stems.includes(s) }"
                  :title="stemName(s)" @click="sp.stems = toggleStem(sp.stems, s)">{{ stemName(s) }}</button>
          <label class="muted" :title="t('preset.edit.level.tip')">
            <input type="range" min="-24" max="24" step="0.5" v-model.number="sp.db" /> {{ sp.db > 0 ? '+' : '' }}{{ sp.db }} {{ t('studio.inserts.dbUnit') }}</label>
          <label class="muted" :title="t('preset.edit.level.tip')">
            <input type="checkbox" :checked="sp.levelDb != null"
                   @change="sp.levelDb = $event.target.checked ? (sp.levelDb ?? 0) : null" /> {{ t('preset.edit.level') }}</label>
          <input v-if="sp.levelDb != null" type="range" min="-40" max="6" step="1" v-model.number="sp.levelDb" />
          <span v-if="sp.levelDb != null" class="muted">{{ sp.levelDb }}</span>
          <PlaceControls v-model="sp.place" />
        </div>
        <details v-if="specKind(sp) === 'engine'" @toggle="toggleEdit(sp, $event.target.open)">
          <summary class="muted">{{ t('preset.edit.spec.chain') }}</summary>
          <ChainEditor v-model="sp.edit" :assets="assets" :busy="busy === 'kit' ? 'kit' : ''"
                       @upload="uploadAsset" @install-kit="installKit" />
        </details>
      </div>

      <PresetCatalog v-model:target="target" :groups="catalog" :assets="assets" :busy="refBusy || busy === 'preview'"
                     @preview="previewItem" @add="addItem" @rhythm="addRhythm" />

      <h3 class="muted">{{ t('preset.edit.master') }}</h3>
      <div class="dsp-row">
        <button v-for="m in MASTERS" :key="m.id" class="ghost small-btn" :class="{ on: m.id === masterPicked }"
                :title="tr(m.note)" @click="pickMaster(m)">{{ tr(m.name) }}</button>
      </div>
      <div class="dsp-row">
        <label class="muted" :title="t('mix.target.tip')">
          <input type="checkbox" :checked="model.targetLufs != null"
                 @change="model.targetLufs = $event.target.checked ? (model.targetLufs ?? -14) : null" /> {{ t('mix.target') }}</label>
        <input v-if="model.targetLufs != null" type="range" min="-24" max="-6" step="0.5" v-model.number="model.targetLufs" />
        <span v-if="model.targetLufs != null" class="muted">{{ model.targetLufs }} LUFS</span>
        <button class="ghost small-btn" :disabled="(!masterEdit.length && model.targetLufs == null) || !!masterMissing.length || refBusy"
                :title="t('mix.master.after.tip')" @click="playMasterAfter">
          {{ busy === 'preview' ? t('instr.busy') : playBtn('pe-master') + ' ' + t('instr.after') }}</button>
      </div>
      <details v-if="masterEdit.length || MASTERS.length">
        <summary class="muted">{{ t('mix.master.edit') }}</summary>
        <ChainEditor v-model="masterEdit" :assets="assets" :busy="busy === 'kit' ? 'kit' : ''"
                     @upload="uploadAsset" @install-kit="installKit" />
      </details>

      <div class="dsp-row">
        <span class="muted">{{ t('preset.save.final') }}</span>
        <VSelect :model-value="finalFrom" :options="finalOptions" searchable style="max-width: 320px"
                 @update:model-value="(v) => { finalFrom = v; pickFinal(v) }" />
        <span v-if="finalLine" class="muted">{{ finalLine }}</span>
      </div>
      <p v-if="(model.parts || []).length" class="muted">{{ t('preset.edit.parts', { n: model.parts.length }) }}</p>
      <p v-if="kindIssues.length" class="err">{{ t('preset.edit.needRoformer') }}</p>

      <p v-if="msg" class="muted">{{ msg }}</p>
      <p v-if="err" class="err">{{ err }}</p>
      <div class="modal-actions">
        <button class="primary" :disabled="!canSave" @click="save">
          {{ action === 'save' ? '…' : isCopy ? t('preset.edit.save.dup') : t('common.save') }}</button>
        <button class="ghost small" :disabled="busy !== '' || action !== '' || applyLive" :title="t('preset.edit.apply.tip')" @click="applyToRef">
          {{ action === 'apply' ? '…' : applyLive ? t('instr.busy') : t('preset.edit.apply') }}</button>
        <span class="spacer"></span>
        <button class="ghost" @click="close">{{ t('common.cancel') }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.preset-editor-modal { width: min(880px, 94vw); overflow-y: auto; }
.preset-note { flex: 1; min-width: 200px; }
.preset-ref { flex-wrap: wrap; align-items: center; gap: 8px; }
.preset-spec { border-left: 2px solid var(--line, #2a2a35); padding-left: 10px; display: flex; flex-direction: column; gap: 4px; margin: 4px 0; }
.preset-spec-line { flex: 1; font-size: 13px; }
.preset-spec-ctrl { flex-wrap: wrap; align-items: center; gap: 6px; }
.stem-chip.on { border-color: var(--accent, #7aa2f7); }
h3 { margin: 8px 0 2px; font-size: 13px; text-transform: none; }
.spacer { flex: 1; }
</style>
