<script setup>
// Страница «Пресеты звука» (меню ⋮): библиотека пресетов — что внутри, свои — переименовать и удалить.
// Применяются пресеты в форме нового трека и в студии («Готово»), здесь — только просмотр и правка своих.
import AppIcon from './AppIcon.vue'
import PageView from './PageView.vue'
import PresetLibrary from './PresetLibrary.vue'
import { ref } from 'vue'
import { useI18n } from '../i18n/index.js'

const { t } = useI18n()
// «Новый пресет» в строке заголовка открывает редактор библиотеки
const lib = ref(null)
</script>

<template>
  <PageView :title="t('nav.presets')">
    <template #actions>
      <button class="primary" :title="t('preset.lib.new.tip')" @click="lib.newPreset()">
        <AppIcon name="plus" /> {{ t('preset.lib.new') }}</button>
    </template>
    <template #lead>{{ t('preset.lib.desc') }}</template>
    <PresetLibrary ref="lib" />
  </PageView>
</template>
