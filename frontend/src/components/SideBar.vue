<script setup>
// Левый блок: логотип с названием, статистика воркера (видеопамять, очереди)
// и общая очередь воркера (генерации и долгие операции приложения и ИИ);
// внизу прижато меню разделов, ещё ниже — тема и язык.
// Сворачивается в узкую полосу с иконками (очередь — счётчиком на «треках»).
import { computed, ref } from 'vue'
import { api } from '../api.js'
import { useI18n } from '../i18n/index.js'
import { useConfirm } from '../composables/useConfirm.js'
import { queueRows } from '../queueView.js'
import { opProgressText } from '../opProgress.js'

const props = defineProps({
  logo: { type: String, default: '' },
  stats: { type: Object, default: null },
  queue: { type: Object, default: () => ({ paused: false, items: [] }) }, // общая очередь воркера
  health: { type: Object, default: null },     // {up, title}
  workerUrl: { type: String, default: '' },    // адрес воркера из настроек
  section: { type: String, default: '' },      // '' — список треков (главная)
  studioJob: { type: Object, default: null },  // открытая студия трека
  theme: { type: String, default: 'dark' },    // 'dark' | 'light'
  locale: { type: String, default: 'ru' },     // 'ru' | 'en'
})
const emit = defineEmits(['nav', 'about', 'theme', 'locale', 'open-job'])
const { t } = useI18n()
const { askConfirm } = useConfirm()

const collapsed = ref((() => { try { return localStorage.getItem('partscaster_sidebar') === '1' } catch { return false } })())
function toggleCollapse() {
  collapsed.value = !collapsed.value
  try { localStorage.setItem('partscaster_sidebar', collapsed.value ? '1' : '0') } catch { /* без localStorage — до закрытия */ }
}

// адрес воркера: где он есть (хост из настроек), рядом со статусом
const workerHost = computed(() => {
  try { return new URL(props.workerUrl).host } catch { return props.workerUrl }
})

// общая очередь воркера: генерации и долгие операции (пресеты, пересборка, стемы,
// эффекты…) от приложения и от ИИ — одним списком: идущая, ждущие, последние итоги
const rows = computed(() => queueRows(props.queue, props.stats))
const liveRows = computed(() => rows.value.filter((r) => r.canCancel))
const queueN = computed(() => liveRows.value.length)
const SHOW_DONE = 5 // завершённых под живыми — последние несколько
const shownRows = computed(() => liveRows.value.concat(rows.value.filter((r) => !r.canCancel).slice(0, SHOW_DONE)))
const paused = computed(() => !!(props.queue && props.queue.paused))
function togglePause() { api.queueSetPaused(!paused.value) }
const stateDot = (r) => ({ waiting: 'queued', running: 'running', done: 'done', error: 'err' }[r.state] || 'off')
const KINDS = ['generate', 'preset', 'rebuild', 'stems', 'fx', 'transcribe', 'whisper', 'voice', 'plan', 'preview', 'render']
// подпись записи: генерация — трек; операция — что делается, над каким треком и чем (пресет)
function rowLabel(r) {
  if (r.kind === 'generate') return '#' + r.jobId + ' ' + (r.title || '')
  const kind = KINDS.includes(r.kind) ? t('pq.kind.' + r.kind) : r.kind
  const extra = r.title && r.title !== r.kind && !r.title.startsWith(r.kind + ' ') ? ' · ' + r.title : ''
  return kind + (r.jobId ? ' #' + r.jobId : '') + extra
}
function resultText(r) {
  const res = r.result
  if (res.type === 'version') return res.jobId ? t('pq.result.version', { id: res.jobId }) : t('pq.result.done')
  return res.type ? t('pq.result.' + res.type) : ''
}
function rowTip(r) {
  if (r.result.type === 'error') return rowLabel(r) + ': ' + r.result.text
  if (r.result.type === 'interrupted') return t('pq.result.interrupted.tip')
  return rowLabel(r)
}
function cancelRow(r) {
  askConfirm(t('pq.cancel.title', { title: rowLabel(r) }), t(r.state === 'running' ? 'pq.cancel.running' : 'pq.cancel.waiting'),
    () => api.queueCancel(r.id))
}
function openResult(r) { if (r.result.type === 'version' && r.result.jobId) emit('open-job', r.result.jobId) }

const NAV = [
  { id: 'new', icon: 'plus' },
  { id: 'tracks', icon: 'music' },
  { id: 'voices', icon: 'mic' },
  { id: 'instruments', icon: 'pedal' },
  { id: 'presets', icon: 'disc' },
  { id: 'settings', icon: 'sliders' },
]
const navTarget = (id) => (id === 'tracks' ? '' : id)
const isActive = (id) => !props.studioJob && props.section === navTarget(id)
function onNav(id) { emit('nav', id) }
</script>

<template>
  <aside class="sidebar" :class="{ collapsed }">
    <!-- логотип и название — одной строкой, без соседей (как в go-sd: div, не кнопка) -->
    <div class="sb-top" :title="t('nav.about.tip')" @click="emit('about')">
      <img v-if="logo" class="app-logo" :src="logo" alt="" width="26" height="26">
      <span v-if="!collapsed" class="sb-title">{{ t('app.title') }}</span>
    </div>

    <!-- статус воркера: где он есть; нет — красная точка и «недоступен» -->
    <div class="sb-worker" :class="{ down: health && !health.up }" :title="health && health.title">
      <span class="health-dot" :class="health && health.up ? 'up' : 'down'"></span>
      <span v-if="!collapsed" class="sb-worker-text">{{ health && health.up ? workerHost : t('sb.worker.down') }}</span>
    </div>

    <!-- линия видеопамяти: занято/всего (что идёт и что ждёт — в очереди ниже) -->
    <div v-if="stats && !collapsed" class="sb-stats">
      <span v-if="stats.vram_total" class="sb-vram" :title="t('footer.vram.tip')">
        <span class="sb-vram-bar">
          <span class="sb-vram-fill" :class="{ hot: stats.vram_used / stats.vram_total > 0.9 }"
                :style="{ width: Math.min(100, 100 * stats.vram_used / stats.vram_total) + '%' }"></span>
        </span>
        <span class="sb-text">{{ (stats.vram_used / 1024).toFixed(1) }}/{{ Math.round(stats.vram_total / 1024) }} {{ t('footer.vram.gb') }}</span>
      </span>
    </div>
    <div v-else-if="stats && collapsed && stats.vram_total" class="sb-stats-mini"
         :title="t('footer.vram.tip') + ': ' + (stats.vram_used / 1024).toFixed(1) + '/' + Math.round(stats.vram_total / 1024) + ' ' + t('footer.vram.gb')"
         :class="{ hot: stats.vram_used / stats.vram_total > 0.9 }">{{ Math.round(100 * stats.vram_used / stats.vram_total) }}%</div>

    <!-- меню разделов; очередь ниже занимает остаток блока -->
    <nav class="sb-nav">
      <button v-for="n in NAV" :key="n.id" class="sb-nav-item" :class="{ on: isActive(n.id) }"
              :title="t('nav.' + n.id + '.tip')" @click="onNav(n.id)">
        <AppIcon class="nav-ico" :name="n.icon" /><span v-if="!collapsed">{{ t('nav.' + n.id) }}</span>
        <span v-if="collapsed && n.id === 'tracks' && queueN" class="sb-badge">{{ queueN > 9 ? '9+' : queueN }}</span>
      </button>
    </nav>

    <!-- общая очередь воркера: что идёт, что дальше и чем кончилось; ✕ у всего незавершённого,
         ▲▼ у ждущих — кнопки видны всегда; пауза — для всех клиентов воркера -->
    <div class="sb-queue" v-if="!collapsed">
      <div class="sb-q-head sb-q-proc-head">
        <span>{{ t('nav.queue') }} <span class="muted">{{ queueN }}</span></span>
        <button class="icon-btn sb-q-pause" :class="{ on: paused }"
                :title="paused ? t('pq.resume') : t('pq.pause')" @click="togglePause"><AppIcon :name="paused ? 'play' : 'pause'" /></button>
      </div>
      <div v-if="shownRows.length" class="sb-q-list">
        <div v-for="r in shownRows" :key="r.id" class="sb-q-item" :class="{ fin: !r.canCancel }" :title="rowTip(r)">
          <span class="sb-q-dot" :class="stateDot(r)"></span>
          <span v-if="r.ai" class="sb-q-ai" :title="t('pq.ai.tip')">{{ t('pq.ai') }}</span>
          <span class="sb-q-title">{{ rowLabel(r) }}</span>
          <span v-if="r.pct != null" class="sb-q-pct">{{ r.pct }}%</span>
          <span v-if="r.canCancel" class="sb-q-ctl">
            <button class="sb-mini" :class="{ hid: !r.canUp }" :disabled="!r.canUp" :title="t('pq.move.up')" @click.stop="api.queueMove(r.id, -1)">▲</button>
            <button class="sb-mini" :class="{ hid: !r.canDown }" :disabled="!r.canDown" :title="t('pq.move.down')" @click.stop="api.queueMove(r.id, 1)">▼</button>
            <button class="sb-mini" :title="t('pq.cancel')" @click.stop="cancelRow(r)">✕</button>
          </span>
          <button v-else-if="r.result.type === 'version' && r.result.jobId" class="sb-mini sb-q-res"
                  :title="t('pq.result.version.tip', { id: r.result.jobId })" @click.stop="openResult(r)">{{ resultText(r) }}</button>
          <span v-else class="sb-q-res" :class="r.result.type">{{ resultText(r) }}</span>
          <!-- ход идущей операции (пресет): шаг, номер сборки, время шага, «осталось ≈» -->
          <span v-if="r.progress" class="sb-q-step">{{ opProgressText(r.progress, t) }}</span>
        </div>
      </div>
      <div v-else class="muted sb-q-empty">{{ t('nav.queue.empty') }}</div>
    </div>

    <!-- нижняя строка: сворачивание, тема, язык -->
    <div class="sb-foot">
      <button class="icon-btn" :title="collapsed ? t('nav.expand') : t('nav.collapse')" @click="toggleCollapse">{{ collapsed ? '»' : '«' }}</button>
      <button class="icon-btn" @click="emit('theme')"
              :title="theme === 'dark' ? t('app.theme.light') : t('app.theme.dark')"><AppIcon :name="theme === 'dark' ? 'sun' : 'moon'" /></button>
      <button class="icon-btn sb-lang" @click="emit('locale')"
              :title="locale === 'ru' ? 'Switch to English' : 'Переключить на русский'">{{ locale === 'ru' ? 'EN' : 'RU' }}</button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  flex: none; width: 216px; display: flex; flex-direction: column; gap: 6px;
  padding: 10px 8px; background: var(--panel); border-right: 1px solid var(--border);
  min-height: 0;
}
.sidebar.collapsed { width: 46px; align-items: stretch; padding: 10px 4px; }
/* логотип: по центру, крупно, с запасом до статуса */
.sb-top { display: flex; align-items: center; justify-content: center; gap: 9px;
  padding-bottom: 10px; cursor: pointer; color: var(--text); }
.sb-title { font-size: 17px; font-weight: 700; letter-spacing: .3px; min-width: 0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* статус и статистика: одна строка 11px, muted, по центру */
.sb-worker { display: flex; align-items: center; justify-content: center; gap: 6px;
  font-size: 11px; color: var(--muted); }
.sb-worker.down .sb-worker-text { color: var(--err, #e06c75); }
.sidebar.collapsed .sb-worker { justify-content: center; }
.sb-stats { display: flex; flex-wrap: wrap; justify-content: center; gap: 4px 6px; font-size: 11px;
  color: var(--muted); }
.sb-stats-mini { font-size: 10px; color: var(--muted); text-align: center; padding: 2px 0; }
.sb-stats-mini.hot { color: var(--err, #e06c75); }
/* меню разделов: 13px, разделитель снизу */
.sb-nav { display: flex; flex-direction: column; gap: 2px;
  border-bottom: 1px solid var(--border); padding-bottom: 8px; }
.sb-nav-item {
  display: flex; align-items: center; gap: 8px; padding: 6px 8px; font-size: 13px;
  background: none; border: 1px solid transparent; border-radius: 6px; color: var(--text);
  cursor: pointer; text-align: left; position: relative; width: 100%;
}
.sb-nav-item:hover { background: var(--panel2); }
.sb-nav-item.on { color: var(--lcd-text, var(--accent)); border-color: var(--lcd-text, var(--accent)); }
.sidebar.collapsed .sb-nav-item { justify-content: center; padding: 6px 0; }
.sb-badge { position: absolute; top: -3px; right: 2px; font-size: 9px; line-height: 1;
  background: var(--accent, #7aa2f7); color: #fff; border-radius: 7px; padding: 2px 4px; }
/* очередь: те же 13px что меню, разделитель сверху — визуально продолжение списка */
.sb-queue { flex: 1 1 auto; min-height: 60px; overflow-y: auto; display: flex; flex-direction: column;
  padding-top: 6px; }
.sb-q-head { display: flex; align-items: center; justify-content: space-between;
  padding: 0 8px 4px; font-size: 13px; color: var(--text); }
.sb-q-head .muted { font-size: 11px; }
.sb-q-list { display: flex; flex-direction: column; gap: 1px; }
.sb-q-item { display: flex; flex-wrap: wrap; align-items: center; gap: 1px 6px;
  padding: 4px 8px; font-size: 12px; min-width: 0; border-radius: 4px; }
.sb-q-item:hover { background: var(--panel2); }
.sb-q-step { flex-basis: 100%; padding-left: 13px; font-size: 11px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sb-q-dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--muted); }
.sb-q-dot.running { background: var(--ok, #7ec16c); animation: sbpulse 1.2s ease-in-out infinite; }
.sb-q-dot.queued { background: var(--muted); }
.sb-q-dot.done { background: var(--ok, #7ec16c); }
.sb-q-dot.err { background: var(--err, #e06c75); }
.sb-q-dot.off { background: var(--muted); opacity: .4; }
.sb-q-pause { padding: 0 4px; font-size: 10px; }
.sb-q-pause.on { color: var(--accent, #7aa2f7); }
.sb-q-ctl { flex: none; display: inline-flex; gap: 2px; }
.sb-mini { background: none; border: none; color: var(--muted); cursor: pointer; font-size: 9px;
  padding: 0 2px; line-height: 1; }
.sb-mini:hover:not(:disabled) { color: var(--text); }
.sb-mini.hid { visibility: hidden; }
.sb-q-item.fin { opacity: .75; }
.sb-q-ai { flex: none; font-size: 9px; padding: 0 3px; border-radius: 3px; color: var(--accent, #7aa2f7);
  border: 1px solid currentColor; line-height: 1.3; }
.sb-q-res { flex: none; font-size: 10px; color: var(--muted); }
.sb-q-res.error { color: var(--err, #e06c75); }
button.sb-q-res { color: var(--ok, #7ec16c); font-size: 10px; }
@keyframes sbpulse { 0%, 100% { opacity: .35 } 50% { opacity: 1 } }
.sb-q-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sb-q-pct { flex: none; color: var(--ok, #7ec16c); font-size: 11px; }
.sb-q-more, .sb-q-empty { font-size: 12px; padding: 4px 8px; color: var(--muted); }
/* нижняя строка: сворачивание, тема, язык */
.sb-foot { display: flex; gap: 4px; justify-content: center; padding-top: 6px;
  border-top: 1px solid var(--border); margin-top: auto; }
.sidebar.collapsed .sb-foot { flex-direction: column; align-items: center; }
</style>
