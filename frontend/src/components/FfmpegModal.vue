<script setup>
// «Нужен ffmpeg»: на чистой Windows нет ffmpeg — перемотка и эффекты студии не работали молча.
// Качаем закреплённую сборку в папку данных приложения (система не трогается); у кого ffmpeg
// уже в PATH — окно не появляется. На Linux/macOS предлагаем системный пакет.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'

const emit = defineEmits(['close', 'installed'])
const { t } = useI18n()

const installing = ref(false)
const done = ref(0)
const total = ref(0)
const err = ref('')
let timer = null

const windows = computed(() => typeof navigator !== 'undefined'
  && /win/i.test(navigator.userAgent || navigator.platform || ''))
const pct = computed(() => (total.value > 0 ? Math.min(100, Math.round(100 * done.value / total.value)) : 0))

async function install() {
  err.value = ''
  installing.value = true
  timer = setInterval(async () => {
    try {
      const p = await api.ffmpegProgress()
      done.value = p.done
      total.value = p.total
    } catch { /* опрос прогресса — не критичен */ }
  }, 400)
  try {
    await api.ffmpegInstall()
    emit('installed')
  } catch (e) {
    err.value = t('ffmpeg.fail', { err: String(e) })
  } finally {
    clearInterval(timer)
    installing.value = false
  }
}

onMounted(() => { if (!windows.value) return })
onBeforeUnmount(() => clearInterval(timer))
</script>

<style scoped>
.ffmpeg-progress { display: flex; align-items: center; gap: 8px; margin: 8px 0 }
.ffmpeg-progress .bar { flex: 1; height: 12px; background: var(--panel-2, #2b2b37); border: 1px solid var(--border, #454556); overflow: hidden }
.ffmpeg-progress .fill { height: 100%; width: 0; background: linear-gradient(180deg, #ffd27a, #ffbe3d 45%, #c98a1a); transition: width .3s }
.ffmpeg-modal .row { display: flex; gap: 8px; margin-top: 10px }
</style>

<template>
  <div class="modal-backdrop">
    <div class="modal ffmpeg-modal" style="width: min(460px, 92vw)">
      <div class="modal-head">
        <h3>{{ t('ffmpeg.title') }}</h3>
      </div>
      <p class="muted">{{ t('ffmpeg.text') }}</p>
      <template v-if="windows">
        <div v-if="installing" class="ffmpeg-progress">
          <div class="bar"><div class="fill" :style="{ width: pct + '%' }" /></div>
          <span class="muted">{{ pct }}%</span>
        </div>
        <p v-if="err" class="error">{{ err }}</p>
        <div class="row">
          <span class="grow" />
          <button class="ghost" :disabled="installing" @click="emit('close')">{{ t('ffmpeg.later') }}</button>
          <button class="primary" :disabled="installing" @click="install">
            {{ installing ? t('ffmpeg.installing') : t('ffmpeg.download') }}
          </button>
        </div>
      </template>
      <p v-else class="muted">{{ t('ffmpeg.unixHint') }}</p>
    </div>
  </div>
</template>
