<script setup>
// Пара «было/стало» окна знакомства: клип встроен в приложение, играет встроенный плеер.
// Переключатель во время игры продолжает другую версию с той же секунды (switchStart).
import { ref, computed } from 'vue'
import { api } from '../api.js'
import { useI18n } from '../i18n/index.js'
import { usePlayer, playbackEnded } from '../composables/usePlayer.js'
import { switchStart, WELCOME_CLIP_ID } from '../welcomeSteps.js'

const props = defineProps({ pair: { type: Object, required: true } })
const { t, locale } = useI18n()
const { playerState, nowPlaying, nowPlayingKey, refreshPlayer } = usePlayer()

const which = ref('before')
const err = ref('')
const key = computed(() => 'welcome:' + props.pair.id)
const tr = (o) => (o && (o[locale.value] || o.ru)) || ''
const playing = computed(() => nowPlayingKey.value === key.value && playerState.value.job_id === WELCOME_CLIP_ID
  && playerState.value.playing && !playbackEnded(playerState.value))

async function play(w, start) {
  err.value = ''
  try {
    await api.playWelcomeClip(props.pair[w], start, props.pair.dur)
    nowPlayingKey.value = key.value
    nowPlaying.value = tr(props.pair.title) + ' · ' + t(w === 'before' ? 'welcome.before' : 'welcome.after')
  } catch (e) {
    err.value = t('player.errorDetail', { err: String(e) })
  }
  refreshPlayer()
}

function toggle() {
  if (playing.value) return stop()
  return play(which.value, 0)
}

function pick(w) {
  if (w === which.value) return
  const start = switchStart(playing.value, playerState.value.position_sec, props.pair.dur)
  which.value = w
  if (start !== null) play(w, start)
}

// стоп только своего клипа: чужой трек, запущенный не из окна, не трогаем
async function stop() {
  if (nowPlayingKey.value !== key.value || playerState.value.job_id !== WELCOME_CLIP_ID) return
  try { await api.stopAudio() } catch { /* плеер уже остановлен — нечего делать */ }
  nowPlayingKey.value = ''
  refreshPlayer()
}

</script>

<template>
  <div class="welcome-clip">
    <div class="welcome-clip-title">{{ tr(pair.title) }}</div>
    <!-- сетка: «Слушать» и два равных сегмента «было | стало» — у всех пар шага столбцы совпадают -->
    <div class="welcome-clip-row">
      <button class="ghost small-btn welcome-play" :class="{ stop: playing }" @click="toggle">
        <AppIcon :name="playing ? 'stop' : 'play'" /> {{ playing ? t('welcome.stop') : t('welcome.play') }}</button>
      <div class="welcome-ab" role="group">
        <button class="ghost small-btn" :class="{ on: which === 'before' }" @click="pick('before')">
          <span class="welcome-ab-side">{{ t('welcome.before') }}</span> {{ tr(pair.before_label) }}</button>
        <button class="ghost small-btn" :class="{ on: which === 'after' }" @click="pick('after')">
          <span class="welcome-ab-side">{{ t('welcome.after') }}</span> {{ tr(pair.after_label) }}</button>
      </div>
    </div>
    <p v-if="err" class="muted">{{ err }}</p>
  </div>
</template>

<style>
.welcome-clip { display: flex; flex-direction: column; gap: 6px; padding: 10px 0; border-top: 1px solid var(--border); }
.welcome-clip:first-child { border-top: none; }
.welcome-clip-title { font-weight: 600; }
.welcome-clip-row { display: grid; grid-template-columns: 112px 1fr; align-items: stretch; gap: 8px; }
.welcome-play { justify-content: center; }
.welcome-ab { display: grid; grid-template-columns: 1fr 1fr; }
.welcome-ab button { justify-content: flex-start; text-align: left; white-space: normal; line-height: 1.3; }
.welcome-ab button:first-child { border-radius: 4px 0 0 4px; }
.welcome-ab button:last-child { border-radius: 0 4px 4px 0; margin-left: -1px; }
.welcome-ab .on { color: var(--text); border-color: var(--accent); position: relative; z-index: 1; }
.welcome-ab-side { color: var(--muted); margin-right: 4px; }
.welcome-ab .on .welcome-ab-side { color: var(--accent-text); }
</style>
