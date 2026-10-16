<script setup>
// Шаг «Голос» студии: голос (карточка примерочной или свои ручки, общий VoiceKnobs) и обработка голоса
// (эффект на дорожку «голос», StudioVoiceFx). Одна «прослушать кусок» на всё выбранное (voiceListenPlan):
// голос — draft-проба куска, эффект — на голосе этого куска, ничего — оригинал с первого спетого места.
// Одна «Применить»: эффект — в черновик трека, голос — новая версия (тот же план, стиль и seed трека, другой
// голос в строке стиля). Seed — трека: seed карточки подобран на стенде примерочной и на другом треке даёт
// другой голос, а заодно и другую музыку.
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { usePlayer } from '../composables/usePlayer.js'
import { sliceAbc } from '../abcEdit.js'
import { jobWatchStep } from '../jobWatch.js'
import { useInserts } from '../composables/useInserts.js'
import { voicePreviewWindow } from '../dspVoice.js'
import { stemFxRequest } from '../requests.js'
import { voiceFxApplied, voiceFxMatch, voiceListenPlan } from '../voiceListen.js'
import {
  PARAMS_VERSION, defaultVoiceParams, normalizeVoiceParams, voiceDescriptor, voiceCardParams,
  swapVocals, hasSungWords, voiceProbe, voiceTrackJob,
} from '../voiceLab.js'
import VSelect from '../VSelect.vue'
import VoiceKnobs from './VoiceKnobs.vue'
import StudioVoiceFx from './StudioVoiceFx.vue'

const props = defineProps({
  job: { type: Object, required: true },
  abcFile: { type: String, default: '' },
  chains: { type: Array, default: () => [] },  // цепочки DSP — эффекты голоса из них
  sel: { type: Object, default: null },         // выделение на волне/ролле {from, to}
  cursor: { type: Number, default: 0 },         // курсор воспроизведения, с
  sungAt: { type: Number, default: null },      // начало первого спетого такта по плану, с
})
// built(id) — полная версия готова: студия переносит на неё вклейки, как у пересборки
// applied — эффект ушёл в черновик (студия обновляет список версий)
const emit = defineEmits(['built', 'applied'])
const { t } = useI18n()
const { toggleArtifact, playBtn, nowPlayingKey, refreshPlayer } = usePlayer()
const inserts = useInserts()
const fxSel = ref(null)             // выбранная обработка голоса {chain, params} или null
const probeKey = ref('')            // голос, которым спета проба (ключ ручек): тот же — новый рендер не нужен
const playOnDone = ref(false)       // проба запущена кнопкой «прослушать кусок» — по готовности играет сама
const STEM = 'vocals'
const SLOT = 'V'                    // свой слот превью: не путается с «Эффектами» и «Педалями»

const voices = ref([])
const selId = ref('')
const own = ref(false)              // «подобрать своё»: голос из ручек, а не из карточки
const params = ref(defaultVoiceParams())
const probeJob = ref(null)          // {id, status, …} draft-пробы
const probeParams = ref(null)       // ручки, с которыми проба ушла: сохраняется именно её голос
const probeOwn = ref(false)         // проба спета своими ручками — только такую можно сохранить новой карточкой
const probePlan = ref(undefined)    // кусок плана для пробы: undefined — план ещё не подкачан, null — нот голоса нет
const fullJob = ref(null)
const busy = ref(false)
const err = ref('')
const saveName = ref('')
let baseAbc = null
// тактов в пробе: блок текста — целый куплет, и на 8 тактах (#216, 143 BPM) помещались 2 строки из 4 —
// модель втискивала лишние слоги. Черновик всё равно обрывается на ~18 с (бюджет воркера 450)
const PROBE_BARS = 16
let timer = null

onMounted(async () => {
  try { voices.value = (await api.voices()) || [] } catch (e) { err.value = String(e) }
})
onBeforeUnmount(stopPoll)

const card = computed(() => voices.value.find((v) => String(v.id) === selId.value) || null)
const cardOptions = computed(() => voices.value.map((v) => ({ value: String(v.id), label: v.name })))
const curParams = computed(() => (own.value ? normalizeVoiceParams(params.value) : card.value ? voiceCardParams(card.value) : null))
const voiceName = computed(() => (own.value ? t('studio.voice.ownName') : card.value ? card.value.name : ''))
const newStyle = computed(() => (curParams.value ? swapVocals(props.job.style || '', voiceDescriptor(curParams.value)) : ''))

// почему рендер недоступен: без слов петь нечего, без плана нечего держать
const blocker = computed(() => {
  if (!hasSungWords(props.job.lyrics)) return t('studio.voice.noWords')
  if (!props.abcFile) return t('studio.voice.noPlan')
  return ''
})
const canRun = computed(() => !blocker.value && !!curParams.value && !busy.value)
const voiceKey = computed(() => (curParams.value ? JSON.stringify(curParams.value) : ''))
const listenPlan = computed(() => voiceListenPlan({
  voice: !!curParams.value, fx: !!fxSel.value,
  probeReady: !!(probeJob.value && probeJob.value.status === 'done'),
  probePending: active(probeJob),
  probeVoiceKey: probeKey.value, curVoiceKey: voiceKey.value,
}))
// рендер пробы — только если петь есть что и где; эффект на голосе трека и оригинал — всегда
const canListen = computed(() => !busy.value && (listenPlan.value !== 'render' || (!blocker.value && !!probePlan.value)))
// тот же эффект (цепочка, ручки, окно) уже в черновике трека — повторное «Применить» его не дублирует; выбор
// остаётся подсвеченным. Сверка — с самим списком: убрали эффект из «Изменений трека» — снова можно применить
const fxToApply = computed(() => !!fxSel.value && !voiceFxApplied(inserts.appliedFor(props.job.id), fxSel.value,
  stemFxRequest(STEM, fxSel.value.chain, fxSel.value.params, props.sel)))
const canApply = computed(() => !busy.value && (fxToApply.value || canRun.value))
// план подкачиваем сразу: без нот голоса «прослушать кусок» недоступна до нажатия, а не после.
// Следим за abcFile (партитуру могут создать в этой же студии) и за текстом/блокировкой: у версии-варианта
// текст приходит позже — когда студия загрузит список треков и найдёт исходный трек (renderJob)
watch(() => [props.abcFile, props.job.lyrics, blocker.value], async () => {
  if (blocker.value) return
  baseAbc = null
  try { probePlan.value = voiceProbe(await ensureAbc(), props.job.lyrics, PROBE_BARS) } catch (e) { err.value = String(e) }
}, { immediate: true })

async function ensureAbc() {
  if (!baseAbc) baseAbc = await api.jobAbcText(props.job.id, props.abcFile || 'score.abc')
  return baseAbc
}

function stopPoll() {
  if (timer) { clearInterval(timer); timer = null }
}
function startPoll() {
  if (!timer) timer = setInterval(poll, 3000)
  poll()
}
const active = (r) => r.value && ['queued', 'running'].includes(r.value.status)

async function poll() {
  try {
    const jobs = await api.jobs()
    const p = jobWatchStep(probeJob.value, jobs)
    if (p) {
      probeJob.value = p.next
      // проба готова — включаем сами (с выбранной обработкой): «прослушать» и должно звучать само
      if (p.done && playOnDone.value) { playOnDone.value = false; await playProbe() }
    }
    const f = jobWatchStep(fullJob.value, jobs)
    if (f) {
      fullJob.value = f.next
      if (f.done) emit('built', f.row.id)
    }
  } catch (e) { err.value = String(e) }
  if (!active(probeJob) && !active(fullJob)) stopPoll()
}

async function submit(draft) {
  busy.value = true
  err.value = ''
  try {
    const full = await ensureAbc()
    let abc, lyrics
    if (draft) {
      const probe = voiceProbe(full, props.job.lyrics, PROBE_BARS)
      if (!probe) return
      ;({ abc, lyrics } = probe)
    } else {
      // план режем по фактической длительности аудио — как пересборка (полный план бывает длиннее трека)
      const dur = Number(props.job.duration_sec) || 0
      abc = dur > 0 ? sliceAbc(full, 0, dur + 1, 0) : full
      lyrics = props.job.lyrics
    }
    const title = draft
      ? t('studio.voice.probeTitle', { name: voiceName.value })
      : t('studio.voice.trackTitle', { title: props.job.title || t('common.track'), name: voiceName.value })
    const used = curParams.value
    const id = await api.submit(voiceTrackJob(props.job, used, { abc, lyrics, title, draft }))
    if (draft) {
      probeJob.value = { id, status: 'queued' }
      probeParams.value = used
      probeOwn.value = own.value
      probeKey.value = JSON.stringify(used)
    } else {
      fullJob.value = { id, status: 'queued' }
    }
    startPoll()
  } catch (e) { err.value = String(e) } finally { busy.value = false }
}

// эффект на голосе в куске job [from, to] — превью на ПК (было/стало), играет «стало»
async function playFxOn(jobId, from, to) {
  await api.fxPreview(jobId, STEM, [{ chain: fxSel.value.chain, params: { ...fxSel.value.params } }], from, to, SLOT)
  await api.playPreview(jobId, SLOT, 'wet', 0)
  nowPlayingKey.value = 'voicefx'
  refreshPlayer()
}
// готовая проба: с обработкой — эффект на всём куске пробы, без неё — как есть
async function playProbe() {
  const p = probeJob.value
  if (fxSel.value) await playFxOn(p.id, 0, Number(p.duration_sec) || 18)
  else await toggleArtifact('vp' + p.id, p.title, () => api.playAudio(p.id))
}

// одна кнопка «прослушать кусок»: что именно играть — voiceListenPlan
async function listen() {
  err.value = ''
  const plan = listenPlan.value
  if (plan === 'wait') {
    playOnDone.value = true   // проба этим голосом уже идёт — сыграет, как будет готова
    return
  }
  if (plan === 'render') {
    playOnDone.value = true
    await submit(true)
    return
  }
  busy.value = true
  try {
    if (plan === 'probe') await playProbe()
    else {
      const w = voicePreviewWindow(props.sel, props.sungAt, props.cursor || null, props.job.duration_sec)
      if (plan === 'trackFx') await playFxOn(props.job.id, w.from, w.to)
      else {
        // ничего не выбрано — оригинал трека с того же места (первый спетый такт или выделение)
        await toggleArtifact('vo' + props.job.id, props.job.title, async () => {
          await api.playAudio(props.job.id)
          if (w.from > 0) await api.seekAudio(w.from)
        })
      }
    }
  } catch (e) { err.value = String(e) } finally { busy.value = false }
}

// одна «Применить»: эффект — в черновик трека (шаг «Голос»), голос — новая версия трека этим голосом
// (рендер; правки трека, включая этот эффект, переносятся на неё — built → carryTo)
async function apply() {
  err.value = ''
  const withVoice = curParams.value && canRun.value
  busy.value = true
  try {
    if (fxToApply.value) {
      const req = stemFxRequest(STEM, fxSel.value.chain, fxSel.value.params, props.sel)
      // тот же эффект уже записан, но выключен — включаем его, а не кладём второй
      const off = voiceFxMatch(inserts.appliedFor(props.job.id), fxSel.value, req)
      if (off) await inserts.setOff(props.job.id, off.childId, false)
      else await inserts.addStemFx(props.job.id, { ...req, step: 'voice' })
      emit('applied')
    }
  } catch (e) {
    // эффект не записан — и голос не рендерим: версия ушла бы без эффекта, который человек выбрал
    err.value = String(e)
    return
  } finally { busy.value = false }
  // submit сам ведёт busy
  if (withVoice) await submit(false)
}

const canSave = computed(() => own.value && probeOwn.value && saveName.value.trim() && probeParams.value
  && probeJob.value && probeJob.value.status === 'done' && !busy.value)

// карточка: голос пробы (её ручки и id), seed — трека, на котором проба спета
async function save() {
  if (!canSave.value) return
  busy.value = true
  err.value = ''
  try {
    await api.voiceCreate(saveName.value.trim(), probeJob.value.id,
      JSON.stringify({ v: PARAMS_VERSION, ...probeParams.value }), Number(props.job.seed) || 0)
    saveName.value = ''
    voices.value = (await api.voices()) || []
  } catch (e) { err.value = String(e) } finally { busy.value = false }
}
</script>

<template>
  <div class="studio-box">
    <div class="studio-box-head"><span><AppIcon name="mic" /> {{ t('studio.voice.title') }}</span> <span class="muted studio-box-hint">{{ t('studio.voice.sub') }}</span></div>
    <div class="studio-box-body">
      <p v-if="blocker" class="muted">{{ blocker }}</p>
      <p v-else-if="probePlan === null" class="muted">{{ t('studio.voice.noVocalPlan') }}</p>
      <div class="dsp-row">
        <VSelect v-model="selId" :options="cardOptions" :disabled="own" style="max-width: 260px"
                 :placeholder="voices.length ? t('studio.voice.pick') : t('studio.voice.noCards')" />
        <span v-if="card && !own" class="muted voice-card-desc" :title="voiceDescriptor(voiceCardParams(card))">{{ voiceDescriptor(voiceCardParams(card)) }}</span>
        <button class="toggle small-btn" :class="{ on: own }" @click="own = !own">
          <AppIcon name="sliders" /> {{ t('studio.voice.own') }}
        </button>
      </div>
      <template v-if="own">
        <VoiceKnobs v-model="params" />
      </template>
      <p v-if="newStyle" class="muted voice-preview" :title="t('studio.voice.style.tip')">{{ t('studio.voice.style') }}: {{ newStyle }}</p>

      <StudioVoiceFx v-model="fxSel" :chains="chains" />

      <div class="dsp-row voice-actions">
        <button class="ghost" :disabled="!canListen" :title="t('studio.voice.listen.tip')" @click="listen">
          <AppIcon name="play" /> {{ t('studio.voice.probe') }}
        </button>
        <span class="spacer"></span>
        <button class="primary" :disabled="!canApply" :title="t('studio.voice.apply.tip')" @click="apply">
          {{ t('studio.voice.applyAll') }}
        </button>
      </div>

      <p v-if="probeJob" class="trick-hint" :class="probeJob.status === 'error' ? 'error' : 'muted'">
        <template v-if="['queued', 'running'].includes(probeJob.status)">
          <span class="pulse">⟳</span> {{ t('studio.voice.probe.label') }} #{{ probeJob.id }} · {{ t('queue.status.' + probeJob.status) }} · {{ t('voicelab.audition.wait') }}<template v-if="probeJob.elapsed_s"> {{ Math.round(probeJob.elapsed_s) }} {{ t('common.sec') }}</template>
        </template>
        <template v-else-if="probeJob.status === 'done'">
          ✓ {{ t('studio.voice.probe.label') }} #{{ probeJob.id }}
          <button class="ghost small-btn" @click="toggleArtifact('vp' + probeJob.id, probeJob.title, () => api.playAudio(probeJob.id))">{{ playBtn('vp' + probeJob.id) }}</button>
        </template>
        <template v-else-if="probeJob.status === 'error'">{{ probeJob.error }}</template>
      </p>
      <div v-if="own && probeOwn && probeJob && probeJob.status === 'done'" class="corpus-new">
        <input v-model="saveName" :placeholder="t('voicelab.save.name')" @keyup.enter="save" />
        <button class="ghost" :disabled="!canSave" :title="t('studio.voice.save.tip')" @click="save"><AppIcon name="save" /> {{ t('voicelab.save') }}</button>
      </div>

      <p v-if="fullJob" class="trick-hint" :class="fullJob.status === 'error' ? 'error' : 'muted'">
        <template v-if="['queued', 'running'].includes(fullJob.status)">
          <span class="pulse">⟳</span> {{ t('studio.voice.apply.label') }} #{{ fullJob.id }} · {{ t('queue.status.' + fullJob.status) }} · {{ t('studio.voice.apply.wait') }}<template v-if="fullJob.elapsed_s"> {{ Math.round(fullJob.elapsed_s) }} {{ t('common.sec') }}</template><template v-if="fullJob.progress_pct != null"> · {{ fullJob.progress_pct }}%</template>
        </template>
        <template v-else-if="fullJob.status === 'done'">
          ✓ {{ t('studio.voice.apply.label') }} #{{ fullJob.id }} — {{ t('studio.voice.apply.done') }}
          <button class="ghost small-btn" @click="toggleArtifact('vf' + fullJob.id, fullJob.title, () => api.playAudio(fullJob.id))">{{ playBtn('vf' + fullJob.id) }}</button>
        </template>
        <template v-else-if="fullJob.status === 'error'">{{ fullJob.error }}</template>
      </p>
      <p v-if="err" class="error">{{ err }}</p>
    </div>
  </div>
</template>
