<script setup>
// Левый блок: логотип с названием, статистика воркера (видеопамять, очереди)
// и очередь генераций; внизу прижато меню разделов, ещё ниже — тема и язык.
// Сворачивается в узкую полосу с иконками (очередь — счётчиком на «треках»).
import { computed, ref } from 'vue'
import { api } from '../api.js'
import { useI18n } from '../i18n/index.js'

const props = defineProps({
  logo: { type: String, default: '' },
  stats: { type: Object, default: null },
  jobs: { type: Array, default: () => [] },
  queue: { type: Object, default: () => ({ paused: false, items: [] }) }, // очередь обработки (Go)
  health: { type: Object, default: null },     // {up, title}
  workerUrl: { type: String, default: '' },    // адрес воркера из настроек
  section: { type: String, default: '' },      // '' — список треков (главная)
  studioJob: { type: Object, default: null },  // открытая студия трека
  theme: { type: String, default: 'dark' },    // 'dark' | 'light'
  locale: { type: String, default: 'ru' },     // 'ru' | 'en'
})
const emit = defineEmits(['nav', 'home', 'import-recipes', 'about', 'theme', 'locale'])
const { t } = useI18n()

const collapsed = ref((() => { try { return localStorage.getItem('partscaster_sidebar') === '1' } catch { return false } })())
function toggleCollapse() {
  collapsed.value = !collapsed.value
  try { localStorage.setItem('partscaster_sidebar', collapsed.value ? '1' : '0') } catch { /* без localStorage — до закрытия */ }
}

// адрес воркера: где он есть (хост из настроек), рядом со статусом
const workerHost = computed(() => {
  try { return new URL(props.workerUrl).host } catch { return props.workerUrl }
})

// очередь генераций: идущие и ждущие джобы воркера
const activeJobs = computed(() => (props.jobs || []).filter((j) => j.status === 'queued' || j.status === 'running'))
const queueN = computed(() => activeJobs.value.length)
const runningPct = computed(() => {
  const r = props.stats && props.stats.running
  return r && r.progress_pct != null ? r.progress_pct : null
})
// очередь обработки (Go): применение пресетов и другие долгие прогоны
const procItems = computed(() => (props.queue && props.queue.items) || [])
const procPaused = computed(() => !!(props.queue && props.queue.paused))
const procActiveN = computed(() => procItems.value.filter((it) => it.state === 'waiting' || it.state === 'running').length)
function togglePause() { api.queueSetPaused(!procPaused.value) }
const stateDot = (it) => ({ waiting: 'queued', running: 'running', done: 'done', error: 'err', canceled: 'off' }[it.state] || 'off')

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
    <div class="sb-top" :title="t('nav.home.tip')" @click="emit('home')">
      <img v-if="logo" class="app-logo" :src="logo" alt="" width="26" height="26">
      <span v-if="!collapsed" class="sb-title">{{ t('app.title') }}</span>
    </div>

    <!-- статус воркера: где он есть; нет — красная точка и «недоступен» -->
    <div class="sb-worker" :class="{ down: health && !health.up }" :title="health && health.title">
      <span class="health-dot" :class="health && health.up ? 'up' : 'down'"></span>
      <span v-if="!collapsed" class="sb-worker-text">{{ health && health.up ? workerHost : t('sb.worker.down') }}</span>
    </div>

    <!-- линия видеопамяти (и очереди воркера, текущей джобы) -->
    <div v-if="stats && !collapsed" class="sb-stats">
      <span v-if="stats.vram_total" class="sb-vram" :title="t('footer.vram.tip')">
        <span class="sb-vram-bar">
          <span class="sb-vram-fill" :class="{ hot: stats.vram_used / stats.vram_total > 0.9 }"
                :style="{ width: Math.min(100, 100 * stats.vram_used / stats.vram_total) + '%' }"></span>
        </span>
        <span class="sb-text">{{ (stats.vram_used / 1024).toFixed(1) }}/{{ Math.round(stats.vram_total / 1024) }} {{ t('footer.vram.gb') }}</span>
      </span>
      <span v-if="stats.queue && stats.queue.queued" class="sb-text">· {{ t('footer.queue', { n: stats.queue.queued }) }}</span>
      <span v-if="stats.gpu_waiting" class="sb-text">· {{ t('footer.gpu_queue', { n: stats.gpu_waiting }) }}</span>
      <span v-if="stats.running" class="sb-text sb-job" :title="stats.running.title">
        · #{{ stats.running.job_id }}
        <template v-if="stats.running.progress_pct != null">{{ stats.running.progress_pct }}%</template>
        <template v-else>{{ t('queue.progress.' + (stats.running.stage || 'plan')) }}</template>
        <template v-if="stats.running.tok_per_s"> · {{ stats.running.tok_per_s }} {{ t('queue.progress.tps') }}</template>
      </span>
    </div>
    <div v-else-if="stats && collapsed && stats.vram_total" class="sb-stats-mini"
         :title="t('footer.vram.tip') + ': ' + (stats.vram_used / 1024).toFixed(1) + '/' + Math.round(stats.vram_total / 1024) + ' ' + t('footer.vram.gb')"
         :class="{ hot: stats.vram_used / stats.vram_total > 0.9 }">{{ Math.round(100 * stats.vram_used / stats.vram_total) }}%</div>

    <!-- открытая студия: какой трек правим (выход — любой раздел или логотип) -->
    <div v-if="studioJob && !collapsed" class="sb-studio" :title="'#' + studioJob.id + ' ' + studioJob.title">
      студия · #{{ studioJob.id }} {{ studioJob.title }}
    </div>

    <!-- меню разделов; очередь ниже занимает остаток блока -->
    <nav class="sb-nav">
      <button v-for="n in NAV" :key="n.id" class="sb-nav-item" :class="{ on: isActive(n.id) }"
              :title="t('nav.' + n.id + '.tip')" @click="onNav(n.id)">
        <AppIcon class="nav-ico" :name="n.icon" /><span v-if="!collapsed">{{ t('nav.' + n.id) }}</span>
        <span v-if="collapsed && n.id === 'tracks' && queueN" class="sb-badge">{{ queueN > 9 ? '9+' : queueN }}</span>
      </button>
    </nav>

    <!-- очередь генераций: что сейчас и что дальше -->
    <div class="sb-queue" v-if="!collapsed">
      <div class="sb-q-head">{{ t('nav.queue') }} <span class="muted">{{ queueN }}</span></div>
      <div v-if="queueN" class="sb-q-list">
        <div v-for="j in activeJobs.slice(0, 8)" :key="j.id" class="sb-q-item" :title="'#' + j.id + ' ' + j.title">
          <span class="sb-q-dot" :class="j.status"></span>
          <span class="sb-q-title">#{{ j.id }} {{ j.title }}</span>
          <span v-if="j.status === 'running' && stats && stats.running && stats.running.job_id === j.id && runningPct != null"
                class="sb-q-pct">{{ runningPct }}%</span>
        </div>
        <div v-if="queueN > 8" class="muted sb-q-more">+{{ queueN - 8 }}</div>
      </div>
      <div v-else class="muted sb-q-empty">{{ t('nav.queue.empty') }}</div>

      <!-- обработка: применение пресетов и другие долгие прогоны приложения;
           порядок меняется, ждущие отменяются, очередь ставится на паузу -->
      <div class="sb-q-head sb-q-proc-head">
        <span>{{ t('pq.processing') }} <span class="muted">{{ procActiveN }}</span></span>
        <button class="icon-btn sb-q-pause" :class="{ on: procPaused }"
                :title="procPaused ? t('pq.resume') : t('pq.pause')" @click="togglePause"><AppIcon :name="procPaused ? 'play' : 'pause'" /></button>
      </div>
      <div v-if="procItems.length" class="sb-q-list">
        <div v-for="(it, k) in procItems.slice(0, 8)" :key="it.id" class="sb-q-item"
             :title="it.err ? it.err : it.title + (it.job_id ? ' · #' + it.job_id : '')">
          <span class="sb-q-dot" :class="stateDot(it)"></span>
          <span class="sb-q-title">{{ it.title }}</span>
          <span v-if="it.state === 'waiting'" class="sb-q-ctl">
            <button v-if="k > 1" class="sb-mini" :title="t('pq.move.up')" @click.stop="api.queueMove(it.id, -1)">▲</button>
            <button class="sb-mini" :title="t('pq.cancel')" @click.stop="api.queueCancel(it.id)">✕</button>
          </span>
        </div>
        <div v-if="procItems.length > 8" class="muted sb-q-more">+{{ procItems.length - 8 }}</div>
      </div>
      <div v-else class="muted sb-q-empty">{{ t('nav.queue.empty') }}</div>
    </div>

    <!-- нижняя строка — действия, не разделы: сворачивание, импорт рецепта (файл), о программе, тема, язык -->
    <div class="sb-foot">
      <button class="icon-btn" :title="collapsed ? t('nav.expand') : t('nav.collapse')" @click="toggleCollapse">{{ collapsed ? '»' : '«' }}</button>
      <button class="icon-btn" :title="t('recipe.importMenu.tip')" @click="emit('import-recipes')"><AppIcon name="folder" /></button>
      <button class="icon-btn" :title="t('nav.about.tip')" @click="emit('about')"><AppIcon name="info" /></button>
      <button class="icon-btn" @click="emit('theme')"
              :title="theme === 'dark' ? t('app.theme.light') : t('app.theme.dark')"><AppIcon :name="theme === 'dark' ? 'sun' : 'moon'" /></button>
      <button class="icon-btn sb-lang" @click="emit('locale')"
              :title="locale === 'ru' ? 'Switch to English' : 'Переключить на русский'">{{ locale === 'ru' ? 'EN' : 'RU' }}</button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  flex: none; width: 216px; display: flex; flex-direction: column; gap: 8px;
  padding: 10px 8px; background: var(--panel); border-right: 1px solid var(--border);
  min-height: 0;
}
.sidebar.collapsed { width: 46px; align-items: stretch; padding: 10px 4px; }
.sb-top { display: flex; align-items: center; justify-content: center; gap: 9px;
  padding-bottom: 14px; cursor: pointer; color: var(--text); }
.sb-title { font-size: 17px; font-weight: 700; letter-spacing: .3px; min-width: 0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sb-worker { display: flex; align-items: center; justify-content: center; gap: 6px;
  font-size: 11px; color: var(--muted); }
.sb-worker.down .sb-worker-text { color: var(--err, #e06c75); }
.sidebar.collapsed .sb-worker { justify-content: center; }
.sb-studio { font-size: 11px; color: var(--muted); border-bottom: 1px solid var(--border);
  padding-bottom: 8px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* статистика — те же классы, что были в шапке (глобальные стили App) */
.sb-stats { display: flex; flex-wrap: wrap; justify-content: center; gap: 4px 6px; font-size: 11px;
  color: var(--muted); border-bottom: 1px solid var(--border); padding-bottom: 8px; }
.sb-stats-mini { font-size: 10px; color: var(--muted); text-align: center; padding: 2px 0;
  border-bottom: 1px solid var(--border); }
.sb-stats-mini.hot { color: var(--err, #e06c75); }
/* очередь генераций */
.sb-queue { flex: 1 1 auto; min-height: 60px; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; }
.sb-q-head { font-size: 11px; text-transform: uppercase; letter-spacing: .8px; color: var(--muted); }
.sb-q-list { display: flex; flex-direction: column; gap: 3px; }
.sb-q-item { display: flex; align-items: center; gap: 6px; font-size: 12px; min-width: 0; }
.sb-q-dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--muted); }
.sb-q-dot.running { background: var(--ok, #7ec16c); animation: sbpulse 1.2s ease-in-out infinite; }
.sb-q-dot.queued { background: var(--muted); }
.sb-q-dot.done { background: var(--ok, #7ec16c); }
.sb-q-dot.err { background: var(--err, #e06c75); }
.sb-q-dot.off { background: var(--muted); opacity: .4; }
/* обработка: пауза очереди и мини-кнопки управления записью */
.sb-q-proc-head { display: flex; align-items: center; justify-content: space-between; margin-top: 8px; }
.sb-q-pause { padding: 0 4px; font-size: 10px; }
.sb-q-pause.on { color: var(--accent, #7aa2f7); }
.sb-q-ctl { flex: none; display: none; gap: 2px; }
.sb-q-item:hover .sb-q-ctl { display: inline-flex; }
.sb-mini { background: none; border: none; color: var(--muted); cursor: pointer; font-size: 9px;
  padding: 0 2px; line-height: 1; }
.sb-mini:hover { color: var(--text); }
@keyframes sbpulse { 0%, 100% { opacity: .35 } 50% { opacity: 1 } }
.sb-q-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sb-q-pct { flex: none; color: var(--ok, #7ec16c); font-size: 11px; }
.sb-q-more, .sb-q-empty { font-size: 11px; }
/* меню разделов — над очередью; очередь занимает остаток блока */
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
.sb-foot { display: flex; gap: 4px; justify-content: center; padding-top: 6px;
  border-top: 1px solid var(--border); margin-top: auto; }
.sidebar.collapsed .sb-foot { flex-direction: column; align-items: center; }
</style>
