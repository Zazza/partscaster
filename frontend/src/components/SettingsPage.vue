<script setup>
// Страница настроек: воркер, дорожки, движок генерации, пути воркера.
import PageView from './PageView.vue'
import PageBox from './PageBox.vue'
import { STEMS_AUTO } from '../trackDesk.js'
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from '../i18n/index.js'
const { t } = useI18n()
import { api } from '../api.js'
import VSelect from '../VSelect.vue'
import { workerCheckText } from '../workerCheck.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useWorkerCompat } from '../composables/useWorkerCompat.js'
import { modelOptions as cppModelOptions, modelConfig } from '../engine.js'
const { askConfirm } = useConfirm()

// имя модели — camelCase под kebab-аргумент v-model:server-url родителя:
// «serverURL» (с большой URL) не сопоставлялось с attr «server-url», поле адреса
// было пустым и «Сохранить» падал на serverURL.value.trim()
const serverURL = defineModel('serverUrl', { type: String, default: '' })
const emit = defineEmits(['close', 'saved'])

const onKey = (e) => { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => { window.removeEventListener('keydown', onKey); clearInterval(modelTimer) })

const info = ref(null)      // data_dir, whisper_py — только показать
const err = ref('')
const saved = ref(false)
const workerProbe = ref('') // статус проверки воркера
// у треков «без голоса» сразу делать дорожки — метка, если в инструментал пролез голос
const autoStems = ref(true)
const stemsAuto = ref('studio')        // когда делать дорожки трека: studio | always | manual
// модель генерации (yue2.cpp) и экономный режим
const cppModel = ref('q8')
const cppLowVram = ref(false)
const engineMsg = ref('')               // итог смены модели/её загрузки
const modelLoad = ref(null)             // идущая загрузка модели {name, file, done, total}
let modelTimer = 0
const checkRes = ref(null)              // «Проверить»: { ok, text }
const appVersion = ref('')
const agents = ref(null)                // { mcp_path, mcp_found, url, snippets }
const agentsMsg = ref('')
const copied = ref('')
const COPIED_MS = 2000   // сколько держится галочка «скопировано»

// «Проверить» — воркер по адресу из поля, без сохранения настроек
async function checkWorker() {
  checkRes.value = null
  const url = (serverURL.value || '').trim()
  if (!url) { checkRes.value = { ok: false, text: t('settings.worker.empty') }; return }
  try {
    checkRes.value = workerCheckText(await api.checkWorker(url), null)
  } catch (e) {
    checkRes.value = workerCheckText({ URL: url }, String(e))
  }
}

async function loadAgents() {
  try { agents.value = await api.agentSetup() } catch { agents.value = null }
}

async function copySnippet(key) {
  try {
    await window.navigator.clipboard.writeText(agents.value.snippets[key])
    copied.value = key
    // значок «скопировано» гаснет сам
    setTimeout(() => { if (copied.value === key) copied.value = '' }, COPIED_MS)
  } catch (e) {
    agentsMsg.value = String(e)
  }
}

function connectClaude() {
  askConfirm(t('settings.agents.connect.title'),
    t('settings.agents.connect.body', { cmd: agents.value.snippets.claude_code }), async () => {
      agentsMsg.value = ''
      try {
        await api.connectClaudeCode()
        agentsMsg.value = t('settings.agents.connected')
      } catch (e) {
        agentsMsg.value = String(e)
      }
    })
}

const engineSupported = computed(() => !!(info.value && info.value.cpp_models)) // старый воркер — блока нет
const cppModels = computed(() => cppModelOptions(info.value))
const cppModelInstalled = computed(() => !!cppModels.value.find((o) => o.value === cppModel.value && o.installed))
// рекомендуемая для карты воркера модель и режим — подсказка под выбором (выбрать можно любую)
const recommended = computed(() => (info.value && info.value.cpp_recommended) || null)

function applyEngine(c) {
  cppModel.value = c.cpp_model || 'q8'
  cppLowVram.value = !!c.cpp_low_vram
}

// загрузка модели yue2.cpp: воркер отвечает по окончании, прогресс — опросом
async function downloadModel() {
  engineMsg.value = ''
  const name = cppModel.value
  modelLoad.value = { name, file: '', done: 0, total: 0 }
  clearInterval(modelTimer)
  modelTimer = setInterval(async () => {
    try {
      const p = await api.engineModelProgress()
      if (p && p.name) modelLoad.value = p
    } catch { /* прогресс — подсказка; загрузка идёт и без него */ }
  }, 1000)
  try {
    await api.installEngineModel(name)
    info.value = await api.workerConfig()
    engineMsg.value = t('settings.engine.downloaded', { name: name.toUpperCase() })
  } catch (e) {
    engineMsg.value = t('settings.engine.downloadFailed', { err: String(e) })
  } finally {
    clearInterval(modelTimer)
    modelLoad.value = null
  }
}

const stemsAutoOptions = computed(() => STEMS_AUTO.map((v) => ({ value: v, label: t('settings.stemsAuto.' + v) })))

onMounted(async () => {
  api.appVersion().then((v) => { appVersion.value = v }).catch(() => {})
  loadAgents()
  try {
    const h = await api.status()
    workerProbe.value = h ? t('settings.probe.workerOk') + (h.model_loaded ? ' ' + t('settings.probe.modelLoaded') : '') : t('settings.probe.workerDown')
  } catch {
    workerProbe.value = t('settings.probe.workerDown')
  }
  try {
    const c = await api.workerConfig()
    autoStems.value = c.auto_stems_instrumental !== false
    stemsAuto.value = c.stems_auto || 'studio'
    applyEngine(c)
    info.value = c
  } catch {
    info.value = null
  }
})

async function save() {
  err.value = ''
  saved.value = false
  try {
    const url = serverURL.value.trim()
    await api.setServerURL(url)
    localStorage.setItem('yue_server', url)
    try {
      await api.setWorkerConfig({
        auto_stems_instrumental: autoStems.value,
        stems_auto: stemsAuto.value,
      })
      const c = await api.workerConfig()
      // сервер мог смениться — сведения о нём (в т.ч. есть ли разделение на дорожки) берём из его ответа
      info.value = c
    } catch (e) {
      err.value = t('settings.worker.notApplied', { err: String(e) })
    }
    // модель — отдельным запросом: отказ (модель не скачана) не роняет остальные настройки
    if (engineSupported.value) {
      try {
        await api.setWorkerConfig(modelConfig(cppModel.value, cppLowVram.value))
        engineMsg.value = ''
      } catch (e) {
        engineMsg.value = t('settings.engine.notApplied', { err: String(e) })
      }
      try {
        const c = await api.workerConfig()
        applyEngine(c)
        info.value = c
      } catch { /* показан прежний выбор; ошибка — выше */ }
    }
    saved.value = true
    useWorkerCompat().recheck() // другой воркер — своя версия
    loadAgents() // во фрагментах конфига — новый адрес
    emit('saved')
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <PageView :title="t('nav.settings')">
    <!-- «Сохранить» — главное действие страницы, в строке заголовка; итог — рядом с ним -->
    <template #actions>
      <span v-if="err" class="error">{{ err }}</span>
      <span v-else-if="saved" class="ok">{{ t('settings.saved') }}</span>
      <button class="primary" @click="save"><AppIcon name="save" /> {{ t('common.save') }}</button>
    </template>

    <PageBox :title="t('settings.worker')">
      <div class="set-row">
        <span class="set-label">{{ t('settings.worker.url') }}</span>
        <input v-model="serverURL" placeholder="http://localhost:8091" @keyup.enter="save" />
        <button class="ghost" @click="checkWorker">{{ t('common.check') }}</button>
      </div>
      <div v-if="checkRes" class="set-hint" :class="checkRes.ok ? 'ok' : 'error'">{{ checkRes.text }}</div>
      <div v-else class="set-hint muted">{{ workerProbe }}</div>
      <div class="set-hint muted">{{ t('settings.worker.hint') }}</div>
      <!-- сведения о воркере — в том же блоке, что адрес: про один и тот же воркер -->
      <div class="set-sub">{{ t('settings.info') }} <span class="muted">{{ t('settings.info.readonly') }}</span></div>
      <div v-if="info" class="set-info muted">
        {{ t('settings.info.dataDir') }}: {{ info.data_dir }}<br />
        whisper: {{ info.whisper_available ? info.whisper_py : t('settings.info.whisperMissing') }}<br />
        {{ t('settings.stems') }}: {{ info.roformer_available ? 'BS-Roformer-SW' : t('settings.stems.none') }}
      </div>
      <div v-else class="set-info muted">{{ t('settings.info.workerDown') }}</div>
      <div v-if="appVersion" class="set-info muted">{{ t('settings.appVersion', { v: appVersion }) }}</div>
    </PageBox>

    <PageBox :title="t('settings.checks')">
      <div class="set-row">
        <span>{{ t('settings.stemsAuto') }}</span>
        <VSelect v-model="stemsAuto" :options="stemsAutoOptions" />
      </div>
      <div class="set-hint muted">{{ t('settings.stemsAuto.hint') }}</div>
      <label class="set-row set-check">
        <input type="checkbox" v-model="autoStems" />
        <span>{{ t('settings.autoStems') }}</span>
      </label>
      <div class="set-hint muted">{{ t('settings.autoStems.hint') }}</div>
      <div v-if="info && !info.roformer_available" class="set-hint error">{{ t('settings.stems.missing') }}</div>
    </PageBox>

      <PageBox v-if="engineSupported" :title="t('settings.engine')">
        <div v-if="!info.cpp_available" class="set-hint error">{{ t('settings.engine.unavailable') }}</div>
        <div class="set-row">
          <span>{{ t('settings.engine.model') }}</span>
          <VSelect v-model="cppModel" :options="cppModels" />
          <button v-if="!cppModelInstalled" class="ghost" :disabled="!!modelLoad" @click="downloadModel">{{ t('settings.engine.download') }}</button>
        </div>
        <div v-if="recommended" class="set-hint muted">{{ t(recommended.low_vram ? 'settings.engine.recommendedLow' : 'settings.engine.recommended', { name: recommended.model.toUpperCase(), gb: recommended.vram_mib ? Math.round(recommended.vram_mib / 1024) : '?' }) }}</div>
        <div v-if="modelLoad" class="set-hint muted">{{ t('settings.engine.downloading', { file: modelLoad.file || '…', done: (modelLoad.done / 1e9).toFixed(1), total: (modelLoad.total / 1e9).toFixed(1) }) }}</div>
        <div v-else-if="!cppModelInstalled" class="set-hint muted">{{ t('settings.engine.needDownload') }}</div>
        <label class="set-row set-check">
          <input type="checkbox" v-model="cppLowVram" />
          <span>{{ t('settings.engine.lowVram') }}</span>
        </label>
        <div class="set-hint muted">{{ t('settings.engine.note') }}</div>
        <div v-if="engineMsg" class="set-hint">{{ engineMsg }}</div>
      </PageBox>

    <PageBox :title="t('settings.agents')" :hint="t('settings.agents.sub')">
      <div class="set-hint muted">{{ t('settings.agents.hint') }}</div>
      <template v-if="agents">
        <div class="set-hint" :class="agents.mcp_found ? 'muted' : 'error'">
          {{ agents.mcp_found ? t('settings.agents.path', { path: agents.mcp_path }) : t('settings.agents.missing', { path: agents.mcp_path }) }}
        </div>
        <div class="set-actions">
          <button :disabled="!agents.mcp_found" @click="connectClaude">{{ t('settings.agents.connect') }}</button>
        </div>
        <div v-if="agentsMsg" class="set-hint">{{ agentsMsg }}</div>
        <div v-for="k in ['claude_code', 'claude_desktop', 'codex']" :key="k" class="agent-snippet">
          <span class="muted">{{ t({ claude_code: 'settings.agents.claudeCode', claude_desktop: 'settings.agents.claudeDesktop', codex: 'settings.agents.codex' }[k]) }}</span>
          <!-- пример конфига — блоком кода, «скопировать» — значком в углу -->
          <div class="code-block">
            <pre>{{ agents.snippets[k] }}</pre>
            <button class="ghost icon code-copy" :title="copied === k ? t('settings.agents.copied') : t('settings.agents.copy')"
                    @click="copySnippet(k)"><AppIcon :name="copied === k ? 'check' : 'copy'" /></button>
          </div>
        </div>
      </template>
    </PageBox>
  </PageView>
</template>

<style>
/* блок кода: моноширинно, с прокруткой вбок, «скопировать» — значком в правом верхнем углу */
.code-block { position: relative; }
.code-block pre { margin: 4px 0 0; padding: 10px 44px 10px 12px; overflow-x: auto; user-select: text;
  background: var(--bg); border: 1px solid var(--border); border-radius: 4px;
  font: 12px/1.5 'DejaVu Sans Mono', 'Consolas', monospace; color: var(--text); }
.code-copy { position: absolute; top: 8px; right: 8px; }
/* подзаголовок внутри блока настроек */
.set-sub { margin-top: 6px; font-size: 13px; font-weight: 600; }
</style>
