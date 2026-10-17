<script setup>
// Полоса заголовка окна вместо системной (main.go: Frameless): название, перетаскивание,
// двойной щелчок — развернуть, кнопки окна справа.
import { WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime.js'
import { windowTitle, maximised, installWindowFrame } from '../composables/useWindowFrame.js'
import { useI18n } from '../i18n/index.js'
import AppIcon from './AppIcon.vue'

const { t } = useI18n()
installWindowFrame()
</script>

<template>
  <div class="titlebar" @dblclick.self="WindowToggleMaximise">
    <span class="titlebar-text" @dblclick="WindowToggleMaximise">{{ windowTitle || t('app.title') }}</span>
    <button class="titlebar-btn" :title="t('window.minimise')" @click="WindowMinimise"><AppIcon name="win-min" /></button>
    <button class="titlebar-btn" :title="maximised ? t('window.restore') : t('window.maximise')" @click="WindowToggleMaximise">
      <AppIcon :name="maximised ? 'win-restore' : 'win-max'" />
    </button>
    <button class="titlebar-btn close" :title="t('window.close')" @click="Quit"><AppIcon name="x" /></button>
  </div>
</template>

<style>
/* рамка окна: края 6 px — ширина зоны растягивания Wails, чтобы видимая рамка и
   зона совпадали; сверху края нет — там полоса заголовка во всю ширину */
#app { padding: 0 6px 6px; background: linear-gradient(var(--bg), var(--bg)) content-box, var(--frame); }
.titlebar { margin: 0 -6px; }
.win-maximised #app { padding: 0; }
/* скругление углов окна (Linux, фон окна прозрачный): contain — чтобы и подложки модалок
   (position: fixed) обрезались по скруглению */
.win-rounded, .win-rounded body { background: transparent; }
.win-rounded #app { border-radius: 8px; overflow: hidden; contain: paint; }
.win-rounded.win-maximised #app { border-radius: 0; }
.win-maximised .titlebar { margin: 0; }
/* перетаскивание окна — свойство Wails; кнопки его не наследуют */
.titlebar { --wails-draggable: drag; position: relative; z-index: 100; flex: none; display: flex; align-items: center;
  height: 32px; background: var(--frame); color: var(--muted); user-select: none; }
/* название — по центру окна (а не полосы без кнопок): по бокам равные поля под кнопки */
.titlebar-text { position: absolute; left: 140px; right: 140px; text-align: center; font-size: 13px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.titlebar-btn:first-of-type { margin-left: auto; }
.titlebar-btn { --wails-draggable: no-drag; flex: none; width: 44px; height: 100%; padding: 0; border: 0; border-radius: 0;
  background: none; box-shadow: none; color: var(--muted); display: flex; align-items: center; justify-content: center; }
.titlebar-btn:hover { background: var(--panel3); color: var(--text); filter: none; }
.titlebar-btn:active:not(:disabled) { background: var(--panel3); transform: none; }
/* светлая тема красит все button стеклом с рамкой (style.css) — кнопкам окна это не нужно */
[data-theme="light"] .titlebar-btn { background: none; box-shadow: none; backdrop-filter: none;
  -webkit-backdrop-filter: none; border: 0; }
[data-theme="light"] .titlebar-btn:hover { background: var(--panel3); }
[data-theme="light"] .titlebar-btn.close:hover { background: var(--err); color: #fff; }
.titlebar-btn.close:hover { background: var(--err); color: #fff; }
</style>
