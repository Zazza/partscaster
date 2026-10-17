<script setup>
// Страница «Пресеты звука»: библиотека пресетов — что внутри, свои — переименовать и удалить; редактор
// пресета открывается вместо библиотеки отдельной страницей (выход — «← К пресетам»/Esc, библиотека перечитывается).
// Применяются пресеты в форме нового трека и в студии («Готово»).
import AppIcon from './AppIcon.vue'
import PageView from './PageView.vue'
import PresetLibrary from './PresetLibrary.vue'
import PresetEditor from './PresetEditor.vue'
import { onMounted, ref } from 'vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'

const { t } = useI18n()
const editing = ref(false)   // false — библиотека; null — новый пресет; объект — правка своего / копия встроенного
// список пресетов для «финал как у…» в редакторе — свой у страницы: редактор можно открыть раньше, чем
// библиотека дочитает, и после сохранения список должен знать новый пресет
const presets = ref([])
async function loadPresets() {
  try { presets.value = (await api.soundPresets()) || [] } catch { /* нет списка — «финал как у…» пуст, библиотека покажет ошибку */ }
}
onMounted(loadPresets)
</script>

<template>
  <PresetEditor v-if="editing !== false" :preset="editing" :presets="presets" @close="editing = false" @saved="loadPresets" />
  <PageView v-else :title="t('nav.presets')">
    <template #actions>
      <button class="primary" :title="t('preset.lib.new.tip')" @click="editing = null">
        <AppIcon name="plus" /> {{ t('preset.lib.new') }}</button>
    </template>
    <template #lead>{{ t('preset.lib.desc') }}</template>
    <PresetLibrary @edit="(p) => { editing = p }" />
  </PageView>
</template>
