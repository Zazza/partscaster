<script setup>
// Инструментальная стойка формы нового трека: группа → инструмент (с поиском) → эффект → «Добавить»;
// добавленное — фишками, ✕ убирает. В строку стиля уходит через rackCompile (App.vue).
import { computed, ref } from 'vue'
import { useI18n } from '../i18n/index.js'
import VSelect from '../VSelect.vue'
import AppIcon from './AppIcon.vue'
import { rackGroups, rackEffects, allRackItems, rackAdd, rackRemove } from '../rack.js'

const sel = defineModel({ type: Array, default: () => [] })   // [{ id, effect }]
const { t, locale } = useI18n()
const nm = (o) => (locale.value === 'en' ? (o?.nameEn || o?.name) : o?.name)

const groupId = ref(rackGroups[0].id)
const itemId = ref('')
const effect = ref('')
const groupOpts = computed(() => rackGroups.map((g) => ({ value: g.id, label: nm(g) })))
const itemOpts = computed(() => (rackGroups.find((g) => g.id === groupId.value)?.items || [])
  .map((i) => ({ value: i.id, label: nm(i) })))
const effectOpts = computed(() => rackEffects.map((e) => ({ value: e.id, label: nm(e) })))
// сменили группу — инструмент другой группы не подходит
function pickGroup(v) { groupId.value = v; itemId.value = '' }

function add() {
  if (!itemId.value) return
  sel.value = rackAdd(sel.value, itemId.value, effect.value)
}
const chips = computed(() => sel.value.map((r) => {
  const it = allRackItems().find((x) => x.id === r.id)
  const eff = r.effect ? rackEffects.find((e) => e.id === r.effect) : null
  return { id: r.id, label: nm(it) + (eff ? ' · ' + nm(eff) : '') }
}))
</script>

<template>
  <div class="rack-picker">
    <div class="rack-row" :title="t('form.rack.tip')">
      <span class="muted rack-cap">{{ t('form.rack') }}:</span>
      <VSelect :model-value="groupId" :options="groupOpts" @update:model-value="pickGroup" />
      <VSelect v-model="itemId" :options="itemOpts" :placeholder="t('form.rack.item')" searchable />
      <VSelect v-model="effect" :options="effectOpts" :title="t('form.rack.effect')" />
      <button class="ghost" :disabled="!itemId" @click="add"><AppIcon name="plus" /> {{ t('form.rack.add') }}</button>
    </div>
    <div v-if="chips.length" class="rack-chips">
      <span v-for="c in chips" :key="c.id" class="rack-chip">{{ c.label }}
        <button class="ghost icon" :title="t('form.rack.remove')" @click="sel = rackRemove(sel, c.id)"><AppIcon name="x" /></button>
      </span>
    </div>
  </div>
</template>

<style>
.rack-picker { display: flex; flex-direction: column; gap: 6px; }
.rack-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.rack-row .vselect { flex: 1 1 0; min-width: 110px; }
.rack-row > button { flex: none; }
.rack-cap { flex: none; font-size: 12px; }
.rack-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.rack-chip { display: inline-flex; align-items: center; gap: 2px; padding: 1px 2px 1px 10px; font-size: 12px;
  border: 1px solid var(--border); border-radius: 12px; background: var(--panel2); }
</style>
