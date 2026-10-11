<script setup>
// Страница настроек: воркер + Ollama + пути воркера.
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from '../i18n/index.js'
const { t } = useI18n()
import { api } from '../api.js'
import VSelect from '../VSelect.vue'
import { workerCheckText } from '../workerCheck.js'
import { useConfirm } from '../composables/useConfirm.js'
import { useWorkerCompat } from '../composables/useWorkerCompat.js'
const { askConfirm } = useConfirm()

const serverURL = defineModel('serverURL')
const emit = defineEmits(['close', 'saved'])

// модалка: Esc закрывает
const onKey = (e) => { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

const ollamaURL = ref('')
const ollamaModel = ref('')
const info = ref(null)      // data_dir, whisper_py — только показать
const models = ref([])      // модели, установленные в Ollama
const err = ref('')
const saved = ref(false)
const workerProbe = ref('') // статус проверки воркера
const ollamaProbe = ref('') // статус проверки Ollama
// у треков «без голоса» сразу делать дорожки — метка, если в инструментал пролез голос
const autoStems = ref(true)
const stemsModel = ref('htdemucs')      // разделение на дорожки: htdemucs | roformer
const roformerOk = ref(false)           // окружение RoFormer установлено на воркере
const checkRes = ref(null)              // «Проверить»: { ok, text }
const appVersion = ref('')
const agents = ref(null)                // { mcp_path, mcp_found, url, snippets }
const agentsMsg = ref('')
const copied = ref('')

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

const stemsOptions = computed(() => [
  { value: 'htdemucs', label: t('settings.stemsModel.fast') },
  { value: 'roformer', label: t('settings.stemsModel.clean'), disabled: !roformerOk.value },
])
const modelOptions = computed(() => {
  const opts = models.value.map((m) => ({ value: m, label: m }))
  if (ollamaModel.value && !models.value.includes(ollamaModel.value)) {
    opts.unshift({ value: ollamaModel.value, label: ollamaModel.value + ' (не в списке)' })
  }
  return opts
})

onMounted(async () => {
  api.appVersion().then((v) => { appVersion.value = v }).catch(() => {})
  loadAgents()
  try {
    const h = await api.status()
    workerProbe.value = h ? 'воркер: доступен' + (h.model_loaded ? ' (модель в памяти)' : '') : 'воркер: недоступен'
  } catch {
    workerProbe.value = 'воркер: недоступен'
  }
  try {
    const c = await api.workerConfig()
    ollamaURL.value = c.ollama_url || ''
    ollamaModel.value = c.ollama_model || ''
    autoStems.value = c.auto_stems_instrumental !== false
    stemsModel.value = c.stems_pref || 'htdemucs'
    roformerOk.value = !!c.roformer_available
    info.value = c
    await checkOllama()
  } catch {
    info.value = null
  }
})

async function checkOllama() {
  ollamaProbe.value = ''
  try {
    const m = await api.ollamaModels(ollamaURL.value.trim())
    models.value = m.models || []
    ollamaProbe.value = `ollama: доступен (${models.value.length} модел.)`
  } catch (e) {
    models.value = []
    ollamaProbe.value = 'ollama: недоступен — ' + e
  }
}

async function save() {
  err.value = ''
  saved.value = false
  try {
    const url = serverURL.value.trim()
    await api.setServerURL(url)
    localStorage.setItem('yue_server', url)
    try {
      await api.setWorkerConfig({
        ollama_url: ollamaURL.value.trim(),
        ollama_model: ollamaModel.value.trim(),
        auto_stems_instrumental: autoStems.value,
        stems_model: stemsModel.value,
      })
      const c = await api.workerConfig()
      ollamaURL.value = c.ollama_url || ''
      ollamaModel.value = c.ollama_model || ''
      // сервер мог смениться — доступность RoFormer и выбор берём из его ответа
      stemsModel.value = c.stems_pref || 'htdemucs'
      roformerOk.value = !!c.roformer_available
      info.value = c
    } catch (e) {
      err.value = 'Ollama-настройки не применены (воркер старой версии или недоступен): ' + e
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
  <div class="modal-backdrop page-backdrop">
    <section class="panel page-modal">
      <div class="page-modal-head">
        <h2>{{ t('settings.title') }}</h2>
        <button class="ghost icon" :title="t('common.close')" @click="emit('close')"><AppIcon name="x" /></button>
      </div>
      <div class="page-modal-body">

      <h3 class="set-h">{{ t('settings.worker') }}</h3>
      <div class="set-row">
        <span class="set-label">{{ t('settings.worker.url') }}</span>
        <input v-model="serverURL" placeholder="http://localhost:8091" @keyup.enter="save" />
        <button class="ghost" @click="checkWorker">{{ t('common.check') }}</button>
      </div>
      <div v-if="checkRes" class="set-hint" :class="checkRes.ok ? 'ok' : 'error'">{{ checkRes.text }}</div>
      <div v-else class="set-hint muted">{{ workerProbe }}</div>
      <div class="set-hint muted">{{ t('settings.worker.hint') }}</div>

      <h3 class="set-h">{{ t('settings.ollama') }} <span class="muted">{{ t('settings.ollama.sub') }}</span></h3>
      <div class="set-row">
        <span class="set-label">{{ t('settings.server') }}</span>
        <input v-model="ollamaURL" placeholder="http://127.0.0.1:11434/api/chat" @change="checkOllama" @keyup.enter="save" />
        <button class="ghost" @click="checkOllama">{{ t('common.check') }}</button>
      </div>
      <div class="set-row">
        <span class="set-label">{{ t('settings.model') }}</span>
        <VSelect v-model="ollamaModel" :options="modelOptions" :placeholder="t('settings.model.ph')" />
        <button class="ghost" @click="checkOllama" :title="t('common.refresh')">{{ t('common.refresh') }}</button>
      </div>
      <div class="set-hint muted">{{ ollamaProbe }}</div>

      <h3 class="set-h">{{ t('settings.checks') }}</h3>
      <label class="set-row set-check">
        <input type="checkbox" v-model="autoStems" />
        <span>{{ t('settings.autoStems') }}</span>
      </label>
      <div class="set-hint muted">{{ t('settings.autoStems.hint') }}</div>

      <div class="set-row">
        <span>{{ t('settings.stemsModel') }}</span>
        <VSelect v-model="stemsModel" :options="stemsOptions" />
      </div>
      <div class="set-hint muted">{{ roformerOk ? t('settings.stemsModel.hint') : t('settings.stemsModel.missing') }}</div>

      <h3 class="set-h">{{ t('settings.info') }} <span class="muted">{{ t('settings.info.readonly') }}</span></h3>
      <div v-if="info" class="set-info muted">
        папка данных: {{ info.data_dir }}<br />
        whisper: {{ info.whisper_available ? info.whisper_py : 'не найден (тексты треков недоступны)' }}<br />
        <template v-if="info.stems_model">{{ t('settings.stemsModel') }}: {{ info.stems_model === 'bs-roformer-sw' ? 'BS-Roformer-SW' : t('settings.stemsModel.demucs') }}</template>
      </div>
      <div v-else class="set-info muted">воркер недоступен — Ollama и пути не показать</div>
      <div v-if="appVersion" class="set-info muted">{{ t('settings.appVersion', { v: appVersion }) }}</div>

      <h3 class="set-h">{{ t('settings.agents') }} <span class="muted">{{ t('settings.agents.sub') }}</span></h3>
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
          <div class="set-row">
            <span class="muted">{{ t({ claude_code: 'settings.agents.claudeCode', claude_desktop: 'settings.agents.claudeDesktop', codex: 'settings.agents.codex' }[k]) }}</span>
            <button class="ghost" @click="copySnippet(k)">{{ copied === k ? t('settings.agents.copied') : t('settings.agents.copy') }}</button>
          </div>
          <pre class="set-info" style="white-space: pre-wrap; word-break: break-all; user-select: text">{{ agents.snippets[k] }}</pre>
        </div>
      </template>

      <div v-if="err" class="error">{{ err }}</div>
      <div v-if="saved" class="ok">сохранено</div>
      <div class="set-actions">
        <button @click="save"><AppIcon name="save" /> {{ t('common.save') }}</button>
      </div>
      </div>
    </section>
  </div>
</template>
