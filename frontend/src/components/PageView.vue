<script setup>
// Каркас страницы раздела: заголовок — тем же словом, что пункт меню, главное действие
// страницы — справа в той же строке, описание — под ними, дальше содержимое.
// Один вид у всех разделов вместо встроенных «страниц-модалок».
defineProps({ title: { type: String, required: true } })
</script>

<template>
  <section class="page">
    <div class="page-head">
      <h1 class="page-title">{{ title }}</h1>
      <div v-if="$slots.actions" class="page-actions"><slot name="actions" /></div>
    </div>
    <p v-if="$slots.lead" class="page-lead"><slot name="lead" /></p>
    <slot />
  </section>
</template>

<style>
.page { display: flex; flex-direction: column; gap: 14px; padding: 18px 24px 28px; min-width: 0; }
.page-head { display: flex; align-items: center; gap: 12px; min-height: 34px; }
h1.page-title { margin: 0; font-size: 20px; font-weight: 600; letter-spacing: 0; text-transform: none;
  color: var(--text); text-shadow: none; }
.page-actions { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.page-lead { margin: -6px 0 0; color: var(--muted); max-width: 90ch; }
/* блоки страницы (PageBox) и раскладки: колонки — слева блоки, справа узкая колонка действий,
   прилипает при прокрутке (на узком окне — под блоками); плитки — блоки в две колонки */
.page-box, .studio-box.page-box { margin-top: 0; overflow: visible; }
.page-box-body { display: flex; flex-direction: column; gap: 10px; }
.page-cols { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 14px; align-items: start; }
.page-main { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.page-side { position: sticky; top: 0; display: flex; flex-direction: column; gap: 12px; }
.page-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(440px, 1fr)); gap: 12px; align-items: start; }
.page-tiles > .page-wide { grid-column: 1 / -1; }
@media (max-width: 1180px) {
  .page-cols { grid-template-columns: minmax(0, 1fr); }
  .page-side { position: static; }
}
/* главное действие страницы: кнопка меню («Импорт») — одной высоты с соседней кнопкой */
.page-actions .menu-pop > summary.ghost-btn { padding: 6px 12px; font-size: 13px; }
</style>
