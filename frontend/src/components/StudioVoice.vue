<script setup>
// Шаг «Голос» студии: тот же план, стиль и seed трека, другой голос в строке стиля —
// полный рендер новой версией или draft-проба куска с голосом. Голос — карточка из
// примерочной или свои ручки (общий VoiceKnobs). Seed — трека: seed карточки подобран
// на стенде примерочной и на другом треке даёт другой голос, а заодно и другую музыку.
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { usePlayer } from '../composables/usePlayer.js'
import { sliceAbc } from '../abcEdit.js'
import { jobWatchStep } from '../jobWatch.js'
import {
  PARAMS_VERSION, defaultVoiceParams, normalizeVoiceParams, voiceDescriptor, voiceCardParams,
  swapVocals, hasSungWords, voiceProbe, voiceTrackJob,
} from '../voiceLab.js'
import VSelect from '../VSelect.vue'
import VoiceKnobs from './VoiceKnobs.vue'

const props = defineProps({
  job: { type: Object, required: true },
  abcFile: { type: String, default: '' },
})
// built(id) — полная версия готова: студия переносит на неё вклейки, как у пересборки
const emit = defineEmits(['built'])
const { t } = useI18n()
const { toggleArtifact, playBtn } = usePlayer()

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
const canProbe = computed(() => canRun.value && !!probePlan.value)
// план подкачиваем сразу: без нот голоса «прослушать кусок» недоступна до нажатия, а не после.
// Следим за abcFile: партитуру могут создать в этой же студии («Создать партитуру»)
watch(() => props.abcFile, async () => {
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
      // проба готова — включаем сами: «прослушать» и должно звучать само
      if (p.done) await toggleArtifact('vp' + p.row.id, p.row.title, () => api.playAudio(p.row.id))
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
    } else {
      fullJob.value = { id, status: 'queued' }
    }
    startPoll()
  } catch (e) { err.value = String(e) } finally { busy.value = false }
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
        <button class="ghost small-btn" :class="{ on: own }" @click="own = !own">
          <AppIcon name="sliders" /> {{ t('studio.voice.own') }}
        </button>
      </div>
      <template v-if="own">
        <VoiceKnobs v-model="params" />
      </template>
      <p v-if="newStyle" class="muted voice-preview" :title="t('studio.voice.style.tip')">{{ t('studio.voice.style') }}: {{ newStyle }}</p>

      <div class="corpus-actions">
        <button class="ghost" :disabled="!canProbe" :title="t('studio.voice.probe.tip')" @click="submit(true)">
          <AppIcon name="play" /> {{ t('studio.voice.probe') }}
        </button>
        <button class="primary" :disabled="!canRun" :title="t('studio.voice.apply.tip')" @click="submit(false)">
          <AppIcon name="mic" /> {{ t('studio.voice.apply') }}
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
