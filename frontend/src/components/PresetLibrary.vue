<script setup>
// Библиотека пресетов звука (страница «Пресеты звука»): что внутри (дорожки и финал), свои —
// переименовать, поправить описание, удалить; редактор (PresetEditor) — полноценная правка рецепта
// с предпрослушиванием на демо-эталоне жанра; «▶ на демо» — применить пресет к демо и послушать.
// Встроенные не меняются: копия («Сохранить как свой») открывается в редакторе.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { useConfirm } from '../composables/useConfirm.js'
import { usePresetDemo } from '../composables/usePresetDemo.js'
import { presetTree, PRESET_FAMILIES } from '../soundPresets.js'
import PresetEditor from './PresetEditor.vue'

const { t } = useI18n()
const { askConfirm } = useConfirm()
const demo = usePresetDemo()
const presets = ref([])
const tree = computed(() => presetTree(presets.value))   // по семьям: Рок, Тяжёлое, Электроника, Поп и другое, Мои
const open = ref(null)       // id раскрытого
const edit = ref(null)       // {id, name, note} правки своего
const err = ref('')
const msg = ref('')
const editorPreset = ref(false)  // false — закрыт; null — новый; объект — правка/копия
const demoBusy = ref(new Set())  // id пресетов, применяемых к демо (каждый — своя кнопка, параллельно)

onMounted(load)
async function load() {
  try { presets.value = (await api.soundPresets()) || [] } catch (e) { err.value = String(e) }
}

const stemName = (s) => t('studio.dsp.target.' + s)
// правка пресета по-человечески: «бас: bass → eq → comp», «голос: soften»
function specLine(s) {
  const what = s.engine ? s.engine.map((b) => b.type).join(' → ')
    : s.steps ? s.steps.filter((x) => !x.off).map((x) => x.chain).join(' → ') : s.chain
  return `${(s.stems || []).map(stemName).join(', ')}: ${what}${s.db ? ` (${s.db > 0 ? '+' : ''}${s.db} ${t('studio.inserts.dbUnit')})` : ''}`
}
const finalLine = (p) => (p.final || []).filter((x) => !x.off).map((x) => x.chain).join(' → ')

async function saveEdit(p) {
  err.value = ''
  try {
    await api.soundPresetUpdate(p.id, { ...p, name: edit.value.name.trim(), note: edit.value.note.trim() })
    edit.value = null
    await load()
  } catch (e) { err.value = String(e) }
}

function remove(p) {
  askConfirm(t('preset.lib.del.title'), p.name, async () => {
    err.value = ''
    try { await api.soundPresetDelete(p.id); await load() } catch (e) { err.value = String(e) }
  })
}

// копия встроенного: редактор открывает его копией в памяти (пометка «(копия)»); на сервере свой пресет появится
// только по «Сохранить» — отмена не оставляет лишней «(копия)», двойной клик не плодит дублей
function makeCopy(p) {
  err.value = ''
  editorPreset.value = p
}

// «послушать на демо»: применить пресет к демо-эталону жанра — версия под демо-джобой
async function playOnDemo(p) {
  err.value = ''
  msg.value = ''
  if (demoBusy.value.has(p.id)) return
  demoBusy.value = new Set([...demoBusy.value, p.id])
  try {
    const fam = p.family || PRESET_FAMILIES[0]
    await demo.ensure(fam)
    const st = await demo.waitReady(fam)
    if (st !== 'ready' || !demo.demos.value[fam]?.jobId) {
      throw new Error(t('preset.edit.demo.err', { err: (demo.demos.value[fam] || {}).err || st }))
    }
    msg.value = t('preset.applying', { name: p.name })
    const child = await api.applySoundPresetWith(demo.demos.value[fam].jobId, p)
    msg.value = t('preset.edit.applied', { id: child })
  } catch (e) { msg.value = ''; err.value = String(e) } finally {
    const s = new Set(demoBusy.value)
    s.delete(p.id)
    demoBusy.value = s
  }
}
</script>

<template>
  <div class="dsp-row">
    <button class="primary small" :title="t('preset.lib.new.tip')" @click="editorPreset = null">
      <AppIcon name="plus" /> {{ t('preset.lib.new') }}</button>
    <span v-if="msg" class="muted">{{ msg }}</span>
  </div>
  <p v-if="!presets.length" class="muted">{{ t('preset.lib.empty') }}</p>
  <details v-for="f in tree" :key="f.family" class="preset-family" :open="f.family === ''">
    <summary><strong>{{ f.label }}</strong> <span class="muted">· {{ f.genres.length }}</span></summary>
  <div v-for="p in f.genres.flatMap((g) => g.items)" :key="p.id" class="preset-row">
    <div class="preset-head">
      <button class="ghost small-btn" @click="open = open === p.id ? null : p.id">{{ open === p.id ? '▾' : '▸' }}</button>
      <template v-if="edit && edit.id === p.id">
        <input v-model="edit.name" maxlength="80" />
        <input v-model="edit.note" maxlength="500" class="preset-note" />
        <button class="primary small" :disabled="!edit.name.trim()" @click="saveEdit(p)">{{ t('common.save') }}</button>
        <button class="ghost small-btn" @click="edit = null">{{ t('common.cancel') }}</button>
      </template>
      <template v-else>
        <strong>{{ p.name }}</strong>
        <span v-if="p.builtin" class="badge">{{ t('preset.lib.builtin') }}</span>
        <span class="muted preset-note">{{ p.note }}</span>
        <button class="ghost small-btn" :disabled="demoBusy.has(p.id)" :title="t('preset.lib.play.tip')"
                @click="playOnDemo(p)"><AppIcon name="play" /></button>
        <button v-if="p.builtin" class="ghost small-btn" :title="t('preset.lib.dup.tip')" @click="makeCopy(p)"><AppIcon name="save" /></button>
        <template v-else>
          <button class="ghost small-btn" :title="t('preset.lib.editFull')" @click="editorPreset = p"><AppIcon name="sliders" /></button>
          <button class="ghost small-btn" :title="t('preset.lib.edit')" @click="edit = { id: p.id, name: p.name, note: p.note || '' }"><AppIcon name="pencil" /></button>
          <button class="ghost small-btn" :title="t('preset.lib.del')" @click="remove(p)"><AppIcon name="x" /></button>
        </template>
      </template>
    </div>
    <div v-if="open === p.id" class="preset-body muted">
      <div v-for="(s, i) in p.specs" :key="i">{{ specLine(s) }}</div>
      <div v-if="finalLine(p)">{{ t('preset.lib.final') }} {{ finalLine(p) }}</div>
      <div v-if="p.target_lufs != null">{{ t('preset.lib.target', { lufs: p.target_lufs }) }}</div>
      <div v-if="p.reference_job_id">{{ t('preset.lib.ref', { id: p.reference_job_id }) }}</div>
    </div>
  </div>
  </details>
  <p v-if="err" class="err">{{ err }}</p>
  <PresetEditor v-if="editorPreset !== false" :preset="editorPreset" :presets="presets"
                @close="editorPreset = false" @saved="load" />
</template>

<style scoped>
.preset-family > summary { cursor: pointer; padding: 4px 0; }
.preset-row { border-bottom: 1px solid var(--line, #2a2a35); padding: 4px 0; }
.preset-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.preset-note { flex: 1; min-width: 160px; font-size: 12px; }
.preset-body { padding: 4px 0 4px 34px; font-size: 12px; display: flex; flex-direction: column; gap: 2px; }
</style>
