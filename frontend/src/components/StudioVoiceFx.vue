<script setup>
// Строка «Обработка» в блоке «Голос трека» (шаг «Голос»): эффекты, уместные на голосе (мегафон, телефон,
// перегруз, слэпбэк + общие с пометкой for_voice), всегда на дорожку «голос». Клик по
// эффекту — сразу превью: выделение, а без него — с первого места, где поют (голосовой
// эффект на вступлении не слышно). Схема та же, что у «Педалей»: свой слот превью,
// «было / стало», «только голос», применение — запись реестра пересборки.
import { computed, ref, watch } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { usePlayer, fmtDur } from '../composables/usePlayer.js'
import { useInserts } from '../composables/useInserts.js'
import { chainDefaults, voiceFxChains, voicePreviewWindow } from '../dspVoice.js'
import { stemFxRequest } from '../requests.js'

const props = defineProps({
  job: { type: Object, required: true },
  chains: { type: Array, default: () => [] },
  sel: { type: Object, default: null },        // выделение на волне/ролле {from, to}
  cursor: { type: Number, default: 0 },        // курсор воспроизведения, с
  sungAt: { type: Number, default: null },     // начало первого спетого такта по плану, с
})
const emit = defineEmits(['applied'])
const { t } = useI18n()
const { nowPlayingKey, playerState, refreshPlayer } = usePlayer()
const inserts = useInserts()

const STEM = 'vocals'
const SLOT = 'V'                    // свой слот превью: не путается с «Эффектами» и «Педалями»
const PLAY_KEY = 'voicefx'          // «сейчас играет превью голоса» в общем плеере

const fx = computed(() => voiceFxChains(props.chains))
const selId = ref('')
const params = ref({})
const prev = ref(null)              // {from, to, which: 'wet'|'dry', hasSolo}
const stale = ref(false)            // ручки сдвинули после рендера куска — следующее «слушать» пересчитает
const solo = ref(false)             // только голос, без музыки
const busy = ref(false)
const msg = ref('')
const cur = computed(() => fx.value.find((c) => c.id === selId.value) || null)
watch(params, () => { if (prev.value) stale.value = true }, { deep: true })

const win = () => voicePreviewWindow(props.sel, props.sungAt, props.cursor || null, props.job.duration_sec)
function curPos() {
  const playing = nowPlayingKey.value === PLAY_KEY && playerState.value.job_id === props.job.id
  return playing ? (playerState.value.position_sec || 0) : 0
}
async function play(which, start = 0) {
  const piece = solo.value && prev.value && prev.value.hasSolo ? which + '_solo' : which
  await api.playPreview(props.job.id, SLOT, piece, start)
  nowPlayingKey.value = PLAY_KEY
  prev.value = { ...prev.value, which }
  refreshPlayer()
}

async function render() {
  const w = win()
  const r = await api.fxPreview(props.job.id, STEM, [{ chain: cur.value.id, params: { ...params.value } }], w.from, w.to, SLOT)
  prev.value = { from: r.from, to: r.to, which: 'wet', hasSolo: !!r.wet_solo }
  stale.value = false
}

// одна кнопка слушать: куска ещё нет или ручки сдвинули — рендер и «с эффектом»;
// иначе переключение с эффектом ⇄ без с той же секунды
async function listen() {
  if (!cur.value) return
  busy.value = true
  msg.value = ''
  try {
    if (!prev.value || stale.value) {
      await render()
      await play('wet')
    } else {
      await play(prev.value.which === 'wet' ? 'dry' : 'wet', curPos())
    }
  } catch (e) { msg.value = String(e) } finally { busy.value = false }
}

// клик по эффекту — выбран (подсвечен) и сразу звучит; повторный клик снимает выбор
function pick(c) {
  prev.value = null
  stale.value = false
  if (selId.value === c.id) { selId.value = ''; return }
  selId.value = c.id
  params.value = chainDefaults(c)
  listen()
}

async function apply() {
  if (!cur.value) return
  busy.value = true
  msg.value = ''
  try {
    // окно — выделение; без него — весь трек
    const res = await inserts.addStemFx(props.job.id, stemFxRequest(STEM, cur.value.id, params.value, props.sel))
    emit('applied')
    msg.value = t('studio.voicefx.applied', { name: cur.value.name })
    if (res && res.variant) await api.playFile(props.job.id, res.variant.file, props.job.duration_sec)
  } catch (e) { msg.value = String(e) } finally { busy.value = false }
}
</script>

<template>
  <div class="voice-fx">
    <div class="voice-presets">
      <span class="muted" :title="t('studio.voicefx.sub')">{{ t('studio.voicefx.title') }}</span>
      <button v-for="c in fx" :key="c.id" class="toggle small-btn" :class="{ on: selId === c.id }"
              :title="c.note" :disabled="busy" @click="pick(c)">{{ c.name }}</button>
    </div>
    <template v-if="cur">
      <p class="muted dsp-note">{{ cur.note }}</p>
      <div class="dsp-row">
        <button class="ghost small-btn" :disabled="busy" :title="t('studio.voicefx.listen.tip')" @click="listen">
          <AppIcon name="play" /> <template v-if="busy">…</template><template v-else-if="!prev || stale || prev.which === 'wet'">{{ t('studio.voicefx.wet') }}</template><template v-else>{{ t('studio.voicefx.dry') }}</template>
        </button>
        <label v-if="prev && prev.hasSolo" class="muted" :title="t('studio.voicefx.solo.tip')">
          <input v-model="solo" type="checkbox" @change="play(prev.which, curPos())" /> {{ t('studio.voicefx.solo') }}
        </label>
        <span v-if="prev" class="muted">{{ fmtDur(prev.from) }}–{{ fmtDur(prev.to) }}</span>
        <button class="primary small" :disabled="busy" :title="t('studio.voicefx.apply.tip')" @click="apply">
          {{ t('studio.voicefx.apply') }}
        </button>
      </div>
      <div class="dsp-params">
        <label v-for="p in cur.params" :key="p.id">
          <span class="dsp-plabel">{{ p.label }}</span>
          <input v-model.number="params[p.id]" type="range" :min="p.min" :max="p.max" :step="p.step" :disabled="busy" />
          <span class="dsp-pval">{{ params[p.id] }}</span>
        </label>
      </div>
    </template>
    <p v-if="msg" class="muted">{{ msg }}</p>
  </div>
</template>

<style scoped>
.voice-fx { margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--border); }
</style>
