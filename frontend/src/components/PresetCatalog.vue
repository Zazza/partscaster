<script setup>
// Каталог готовых цепочек для дорожки в редакторе пресета: табы дорожек, строки «▶ название +
// добавление» (▶ — прослушивание на эталоне, владеет им родитель PresetEditor). Для барабанов —
// блок «ритм-секция набором»: выбор набора и обработки, добавление сразу всеми частями.
import { computed, ref } from 'vue'
import { useI18n } from '../i18n/index.js'
import VSelect from '../VSelect.vue'
import { usePlayer } from '../composables/usePlayer.js'
import { EDITOR_TARGETS } from '../presetEdit.js'
import { DRUM_KITS, DRUM_TREATMENTS } from '../drumKits.js'

const props = defineProps({
  target: { type: String, required: true },          // дорожка-цель (v-model:target)
  groups: { type: Array, default: () => [] },        // editorCatalog(target, …)
  assets: { type: Object, default: () => ({ amps: [], irs: [], kits: [] }) },
  busy: { type: Boolean, default: false },           // эталон готовится (превью недоступно)
})
const emit = defineEmits(['update:target', 'add', 'preview', 'rhythm'])
const { t, locale } = useI18n()
const { playBtn } = usePlayer()

const tr = (lbl) => (lbl && (lbl[locale.value] || lbl.ru)) || ''

// у усилителя без захвата цепочка не считается: без загруженных NAM добавлять нечего
const hasAmp = (p) => (p.chain || []).some((b) => b.type === 'amp')
const ampBlocked = (p) => hasAmp(p) && !(props.assets.amps || []).length
const canPlay = (p) => !ampBlocked(p) && !props.busy

// ритм-секция набором: набор и обработка по умолчанию — как в пульте дорожек
const kit = ref('osdk')
const treat = ref('room')
const kitOptions = computed(() => DRUM_KITS.map((k) => ({ value: k.id, label: tr(k.name) })))
const treatOptions = computed(() => DRUM_TREATMENTS.map((x) => ({ value: x.id, label: tr(x.name) })))
const kitNote = computed(() => tr((DRUM_KITS.find((k) => k.id === kit.value) || {}).note))
</script>

<template>
  <div class="preset-catalog">
    <div class="dsp-row preset-tabs">
      <button v-for="tg in EDITOR_TARGETS" :key="tg" class="ghost small-btn" :class="{ on: tg === target }"
              @click="emit('update:target', tg)">{{ t('studio.dsp.target.' + tg) }}</button>
    </div>
    <div v-if="target === 'drums'" class="dsp-row preset-rhythm">
      <span class="muted">{{ t('preset.edit.rhythm') }}</span>
      <VSelect v-model="kit" :options="kitOptions" :title="kitNote" style="max-width: 200px" />
      <VSelect v-model="treat" :options="treatOptions" style="max-width: 160px" />
      <button class="primary small" :disabled="busy" :title="kitNote"
              @click="emit('rhythm', { kit, treat })">{{ t('preset.edit.rhythm.add') }}</button>
    </div>
    <div v-for="g in groups" :key="g.group" class="preset-catalog-group">
      <div v-if="g.label" class="muted preset-group-label">{{ g.label }}</div>
      <div v-for="p in g.items" :key="p.id" class="dsp-row preset-catalog-row">
        <button class="ghost small-btn" :title="ampBlocked(p) ? t('instr.needAmp') : t('instr.after')"
                :disabled="!canPlay(p)" @click="emit('preview', p)">{{ playBtn('pc:' + p.id) }}</button>
        <span class="preset-catalog-name" :title="tr(p.note)">{{ tr(p.name) }}</span>
        <button class="ghost small-btn" :title="ampBlocked(p) ? t('instr.needAmp') : t('preset.edit.add')"
                :disabled="ampBlocked(p)" @click="emit('add', p)">+</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.preset-catalog { display: flex; flex-direction: column; gap: 4px; }
.preset-tabs { flex-wrap: wrap; }
.preset-tabs .on { border-color: var(--accent, #7aa2f7); }
.preset-rhythm { flex-wrap: wrap; align-items: center; gap: 8px; }
.preset-catalog-group { display: flex; flex-direction: column; gap: 2px; }
.preset-group-label { font-size: 12px; padding-top: 4px; }
.preset-catalog-row { align-items: center; gap: 8px; }
.preset-catalog-name { flex: 1; min-width: 160px; font-size: 13px; cursor: help; }
</style>
