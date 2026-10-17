<script setup>
// Блок страницы раздела — рамка с шапкой, как блоки студии: заголовок (с номером шага, если
// страница идёт по шагам), короткая подсказка, справа — инструменты блока.
defineProps({
  title: { type: String, required: true },
  hint: { type: String, default: '' },
})
</script>

<template>
  <section class="studio-box page-box">
    <div class="studio-box-head">
      <span>{{ title }}</span>
      <span v-if="hint" class="muted studio-box-hint">{{ hint }}</span>
      <span v-if="$slots.tools" class="studio-box-tools"><slot name="tools" /></span>
    </div>
    <div class="studio-box-body page-box-body"><slot /></div>
  </section>
</template>
