<script setup>
// Метрики трека с дельтой до референса/другой джобы.
import { ref, computed } from 'vue'
import { useI18n } from '../i18n/index.js'
const { t } = useI18n()
import { api } from '../api.js'
import VSelect from '../VSelect.vue'

const props = defineProps({ jobs: { type: Array, default: () => [] } })

const open = ref(false)
const job = ref(null)
const title = ref('')     // подзаголовок, если показываем не исходник (DSP-вариант)
const metrics = ref(null)
const busy = ref(false)
const err = ref('')
const refs = ref([])
const cmpTarget = ref('') // '' | 'ref:<id>' | 'job:<id>'
const cmpMetrics = ref(null)
const cmpBusy = ref(false)
const addingRef = ref(false)

// подпись строки — t('metrics.row.' + key); единица 'hz' — t('metrics.unit.hz')
const rows = [
  ['tempo_bpm', 'BPM', 1],
  // громкость по стандарту стримингов (EBU R128): стриминги выравнивают к ~−14 LUFS
  ['lufs', 'LUFS', 1],
  ['lra', 'LU', 1],
  ['true_peak_db', 'dBTP', 1],
  ['crest_db', 'dB', 1],
  ['dyn_range_db', 'dB', 1],
  ['rms_p95_db', 'dBFS', 1],
  ['noise_floor_db', 'dBFS', 1],
  ['signal_noise_db', 'dB', 1],
  ['clip_pct', '%', 2],
  ['centroid_hz', 'hz', 0],
  ['f95_hz', 'hz', 0],
  ['flatness_loud', '', 3],
  ['stereo_corr', '', 2],
  ['bands.bass', '%', 1],
  ['bands.low_mid', '%', 1],
  ['bands.mid', '%', 1],
  ['bands.high', '%', 1],
  ['bands.air', '%', 1],
]

const doneJobs = computed(() =>
  props.jobs.filter((j) => j.status === 'done' && j.id !== (job.value && job.value.id)))

function metricVal(m, key) {
  if (!m) return null
  if (key.startsWith('bands.')) return m.bands ? m.bands[key.slice(6)] : null
  return m[key]
}

function fmtMetric(v, dec, unit) {
  if (v === null || v === undefined) return '—'
  const u = unit === 'hz' ? t('metrics.unit.hz') : unit
  return Number(v).toFixed(dec) + (u ? ' ' + u : '')
}

function deltaStr(key, dec) {
  const a = metricVal(metrics.value, key)
  const b = metricVal(cmpMetrics.value, key)
  if (a == null || b == null) return ''
  const d = Number(a) - Number(b)
  return (d > 0 ? '+' : '') + d.toFixed(dec)
}

function refName(id) {
  return String(id).replace(/^\d+-/, '')
}

async function loadRefs() {
  try { refs.value = (await api.references()) || [] } catch {}
}

async function openFor(j, preset) {
  job.value = j
  title.value = ''
  open.value = true
  err.value = ''
  metrics.value = null
  cmpTarget.value = ''
  cmpMetrics.value = null
  if (preset) {
    // вариант DSP: метрики уже известны, сразу сравнение с исходником
    metrics.value = preset.metrics
    title.value = preset.title
    cmpTarget.value = 'job:' + j.id
    await onCmpChange()
    return
  }
  busy.value = true
  try {
    loadRefs()
    metrics.value = await api.analyze(j.id)
  } catch (e) {
    err.value = String(e)
  } finally {
    busy.value = false
  }
}

async function onCmpChange() {
  cmpMetrics.value = null
  if (!cmpTarget.value) return
  cmpBusy.value = true
  try {
    const [kind, id] = cmpTarget.value.split(':')
    if (kind === 'ref') {
      const r = refs.value.find((x) => x.id === id)
      cmpMetrics.value = r ? r.metrics : null
    } else if (kind === 'job') {
      cmpMetrics.value = await api.analyze(Number(id))
    }
  } catch (e) {
    err.value = String(e)
  } finally {
    cmpBusy.value = false
  }
}

async function addReference() {
  addingRef.value = true
  try {
    const r = await api.addReference()
    if (r) {
      await loadRefs()
      cmpTarget.value = 'ref:' + r.id
      await onCmpChange()
    }
  } finally { addingRef.value = false }
}

defineExpose({ openFor })
</script>

<template>
  <div v-if="open" class="modal-backdrop">
    <div class="modal metrics-modal">
      <div class="modal-head">
        <h2>{{ t('metrics.title') }} — {{ title || ('#' + (job && job.id) + ' ' + (job && job.title)) }}</h2>
        <span class="spacer"></span>
        <button class="ghost" @click="open = false"><AppIcon name="x" /></button>
      </div>
      <p v-if="busy" class="muted">{{ t('metrics.librosa') }}</p>
      <p v-if="err" class="error">{{ err }}</p>
      <template v-if="metrics">
        <div class="cmp-row">
          <span class="muted">{{ t('metrics.deltaTo') }}</span>
          <VSelect v-model="cmpTarget" style="max-width: 260px"
                   :options="[{ value: '', label: t('common.none') },
                              ...refs.map((r) => ({ value: 'ref:' + r.id, label: t('metrics.refPrefix') + ' ' + refName(r.id) })),
                              ...doneJobs.map((jj) => ({ value: 'job:' + jj.id, label: '#' + jj.id + ' ' + jj.title }))]"
                   @update:model-value="onCmpChange()" />
          <button class="ghost" :disabled="addingRef" @click="addReference"><AppIcon name="plus" /> {{ t('metrics.addRef') }}</button>
        </div>
        <p v-if="cmpBusy" class="muted">{{ t('metrics.busy') }}</p>
        <div class="metrics-table">
          <div class="mrow head"><span>{{ t('metrics.col.metric') }}</span><span>{{ t('metrics.col.track') }}</span><span>{{ t('metrics.col.cmp') }}</span><span>Δ</span></div>
          <div v-for="[key, unit, dec] in rows" :key="key" class="mrow">
            <span>{{ t('metrics.row.' + key) }}</span>
            <span>{{ fmtMetric(metricVal(metrics, key), dec, unit) }}</span>
            <span class="muted">{{ cmpMetrics ? fmtMetric(metricVal(cmpMetrics, key), dec, unit) : '—' }}</span>
            <span class="delta">{{ deltaStr(key, dec) }}</span>
          </div>
          <div class="mrow">
            <span>{{ t('metrics.row.key') }}</span>
            <span>{{ metrics.key || '—' }}</span>
            <span class="muted">{{ (cmpMetrics && cmpMetrics.key) || '—' }}</span>
            <span></span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
