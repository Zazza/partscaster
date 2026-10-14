<script setup>
// «Мои стили»: все сохранённые свои стили одним списком (из формы и из профилей
// корпусов). Клик — применить в форму, ✕ — удалить с подтверждением.
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from '../i18n/index.js'
import { loadCustomGroups, saveCustomGroups } from '../groups.js'
import { useConfirm } from '../composables/useConfirm.js'

const emit = defineEmits(['close', 'apply'])
const { t } = useI18n()
const { askConfirm } = useConfirm()

const groups = ref(loadCustomGroups())
// один плоский список: имя, группа (для сохранённых из профилей) и строка стиля
const rows = computed(() => groups.value.flatMap((g) => (g.items || []).map((i) => ({
  ...i, gid: g.id, groupName: g.name,
}))))

const onKey = (e) => { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

function remove(row) {
  askConfirm(t('my.del.title', { name: row.name }), t('my.del.body'), () => {
    const g = groups.value.find((x) => x.id === row.gid)
    if (!g) return
    g.items = (g.items || []).filter((i) => i.id !== row.id)
    if (!g.items.length) groups.value = groups.value.filter((x) => x.id !== g.id) // пустая группа не остаётся
    saveCustomGroups(groups.value)
  })
}
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal my-styles-modal">
      <div class="modal-head">
        <h2>{{ t('my.list.title') }} <span class="muted">({{ rows.length }})</span></h2>
        <span class="spacer"></span>
        <button class="ghost" :title="t('common.close')" @click="emit('close')"><AppIcon name="x" /></button>
      </div>
      <div v-if="rows.length" class="my-styles-list">
        <div v-for="r in rows" :key="r.id" class="my-style-row" :title="r.style">
          <button class="ghost my-style-apply" @click="emit('apply', r)">
            <strong>{{ r.name }}</strong>
            <span v-if="r.groupName" class="muted"> · {{ r.groupName }}</span>
          </button>
          <span class="muted my-style-line">{{ (r.style || '').slice(0, 80) }}</span>
          <button class="ghost icon" :title="t('common.delete')" @click="remove(r)"><AppIcon name="x" /></button>
        </div>
      </div>
      <p v-else class="muted">{{ t('my.list.empty') }}</p>
    </div>
  </div>
</template>

<style scoped>
.my-styles-modal { width: min(720px, 92vw); }
.my-styles-list { display: flex; flex-direction: column; gap: 4px; overflow-y: auto; min-height: 0; }
.my-style-row { display: flex; align-items: center; gap: 8px; min-width: 0;
  border-bottom: 1px solid var(--border); padding: 4px 0; }
.my-style-row:last-child { border-bottom: none; }
.my-style-apply { flex: none; max-width: 45%; background: none; border: none; padding: 2px 0;
  text-align: left; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text); }
.my-style-apply:hover { filter: none; text-decoration: underline; }
.my-style-line { flex: 1; min-width: 0; font-size: 12px;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
