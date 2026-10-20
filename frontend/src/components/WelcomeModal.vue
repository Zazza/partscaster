<script setup>
// Окно при запуске: «Что это» по шагам (что за приложение → примеры было/стало → видеокарты)
// и справа «Что нового» (новости версии). Когда показывать — решает welcome.js; шаги примеров
// и их клипы — welcomeClips.json (welcomeSteps.js).
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { api } from '../api.js'
import { useI18n } from '../i18n/index.js'
import { usePlayer } from '../composables/usePlayer.js'
import { RELEASE_NOTES } from '../releaseNotes.js'
import { visibleSteps, WELCOME_CLIP_ID } from '../welcomeSteps.js'
import CLIPS from '../welcomeClips.json'
import WelcomeClip from './WelcomeClip.vue'

const props = defineProps({ tab: { type: String, default: 'about' } })
const emit = defineEmits(['close', 'new-track'])   // close(dontShow), new-track(dontShow)
const { t, locale } = useI18n()
const { playerState, nowPlayingKey, refreshPlayer } = usePlayer()

const cur = ref(props.tab)
watch(() => props.tab, (v) => { cur.value = v })
const dontShow = ref(false)
const notes = RELEASE_NOTES[0]
const steps = visibleSteps(CLIPS)
const step = ref(0)
const st = computed(() => steps[step.value])
const tr = (o) => (o && (o[locale.value] || o.ru)) || ''

// клип окна играет — стоп при смене шага/вкладки и закрытии; чужой трек не трогаем
function stopClip() {
  if (playerState.value.job_id !== WELCOME_CLIP_ID || !nowPlayingKey.value.startsWith('welcome:')) return
  api.stopAudio().catch(() => { /* плеер уже остановлен — нечего делать */ })
  nowPlayingKey.value = ''
  refreshPlayer()
}
watch([step, cur], stopClip)
onBeforeUnmount(stopClip)

const go = (i) => { step.value = Math.max(0, Math.min(steps.length - 1, i)) }
const close = () => emit('close', dontShow.value)
const newTrack = () => emit('new-track', dontShow.value)
</script>

<template>
  <div class="modal-backdrop" @keydown.esc="close">
    <div class="modal welcome-modal" role="dialog" aria-modal="true">
      <div class="modal-head welcome-head">
        <img class="welcome-logo" src="../assets/logo.png" alt="" width="32" height="32">
        <div class="welcome-tabs">
          <button class="ghost small-btn" :class="{ on: cur === 'about' }" @click="cur = 'about'">{{ t('welcome.tab.about') }}</button>
          <button v-if="notes" class="ghost small-btn" :class="{ on: cur === 'news' }" @click="cur = 'news'">
            {{ t('welcome.tab.news') }} · {{ notes.version }}</button>
        </div>
        <button class="ghost icon" :title="t('common.close')" @click="close"><AppIcon name="x" /></button>
      </div>

      <div v-if="cur === 'about'" class="welcome-body">
        <template v-if="st.id === 'about'">
          <h2 class="welcome-title">{{ t('welcome.title') }}</h2>
          <p class="welcome-lead">{{ t('welcome.about.goal') }}</p>
          <p class="welcome-lead">{{ t('welcome.about.how') }}</p>
          <p class="welcome-lead muted">{{ t('welcome.about.name') }}</p>
          <p class="welcome-lead">{{ t('welcome.about.next') }}</p>
        </template>

        <template v-else-if="st.id === 'gpu'">
          <h2 class="welcome-title">{{ t('welcome.gpu.title') }}</h2>
          <p class="welcome-lead">{{ t('welcome.gpu.worker') }}</p>
          <p class="welcome-lead welcome-tight">{{ t('welcome.gpu.models') }}</p>
          <ul class="welcome-news">
            <li>{{ t('welcome.gpu.bf16') }}</li>
            <li>{{ t('welcome.gpu.q8') }}</li>
            <li>{{ t('welcome.gpu.low') }}</li>
          </ul>
          <p class="welcome-lead welcome-agent muted">{{ t('welcome.gpu.agent') }}</p>
          <button class="primary" @click="newTrack"><AppIcon name="plus" /> {{ t('welcome.newTrack') }}</button>
        </template>

        <template v-else>
          <h2 class="welcome-title">{{ tr(st.title) }}</h2>
          <p v-if="tr(st.lead)" class="welcome-lead">{{ tr(st.lead) }}</p>
          <p class="muted welcome-hint">{{ t('welcome.example.hint') }}</p>
          <WelcomeClip v-for="p in st.pairs" :key="p.id" :pair="p" />
        </template>
      </div>

      <div v-else class="welcome-body">
        <h2 class="welcome-title">{{ t('welcome.news.title', { v: notes.version }) }}</h2>
        <ul class="welcome-news">
          <li v-for="(item, i) in notes[locale] || notes.ru" :key="i">{{ item }}</li>
        </ul>
      </div>

      <!-- точки шагов — по центру над нижней строкой; листание — кнопками справа внизу -->
      <div v-if="cur === 'about'" class="welcome-dots">
        <button v-for="(s, i) in steps" :key="s.id" class="welcome-dot" :class="{ on: i === step }"
                :title="t('welcome.step', { n: i + 1 })" :aria-label="t('welcome.step', { n: i + 1 })" @click="go(i)"></button>
      </div>

      <div class="modal-actions welcome-actions">
        <label class="muted welcome-dont"><input v-model="dontShow" type="checkbox"> {{ t('welcome.dontShow') }}</label>
        <span class="spacer"></span>
        <template v-if="cur === 'about'">
          <button v-if="step > 0" class="ghost" @click="go(step - 1)">{{ t('welcome.prev') }}</button>
          <button v-if="step < steps.length - 1" class="primary" @click="go(step + 1)">{{ t('welcome.next') }}</button>
          <button v-else class="primary" @click="close">{{ t('welcome.ok') }}</button>
        </template>
        <button v-else class="primary" @click="close">{{ t('welcome.ok') }}</button>
      </div>
    </div>
  </div>
</template>

<style>
.welcome-modal { width: min(620px, 92vw); gap: 14px; }
/* .modal-head из App.vue подключается позже и ставит baseline — картинка вставала
   нижним краем на линию текста и торчала вверх; двойной класс сильнее */
.modal-head.welcome-head { align-items: center; }
.welcome-logo { width: 32px; height: 32px; flex: none; filter: drop-shadow(0 0 6px var(--lcd-glow)); }
.welcome-tabs { display: flex; align-items: center; gap: 6px; flex: 1; min-width: 0; }
.welcome-tabs .on { color: var(--text); border-color: var(--accent); }
/* высота тела — одна на все шаги: кнопки шагов не прыгают при листании */
.welcome-body { overflow-y: auto; min-height: 300px; }
.welcome-modal h2.welcome-title { margin: 0 0 6px; padding: 0; background: none; border: none; text-shadow: none;
  font-size: 18px; text-transform: none; letter-spacing: 0; color: var(--accent-text); }
.welcome-lead { margin: 0 0 12px; line-height: 1.55; }
.welcome-tight { margin-bottom: 6px; }
.welcome-agent { margin-top: 12px; }
.welcome-hint { margin: 0 0 4px; }
.welcome-news { margin: 0; padding-left: 20px; display: flex; flex-direction: column; gap: 6px; line-height: 1.45; }
.welcome-dots { display: flex; justify-content: center; gap: 8px; }
.welcome-dot { width: 10px; height: 10px; padding: 0; border-radius: 50%; border: 1px solid var(--border);
  background: transparent; cursor: pointer; }
.welcome-dot.on { background: var(--accent); border-color: var(--accent); }
.welcome-actions { align-items: center; gap: 8px; margin-top: 0; }
/* «Назад» (ghost) и «Далее» (primary) — одной высоты и ширины: у классов разные отступы */
.welcome-actions button { padding: 7px 16px; line-height: 1.25; min-width: 104px; }
.welcome-dont { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; }
</style>
