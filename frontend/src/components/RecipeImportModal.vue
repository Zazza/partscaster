<script setup>
// Импорт рецепта трека: выбрать файл → превью (название, шагов, модель, предупреждения) → «Импортировать».
// По правилам рецепта создаётся НОВЫЙ трек; ведёт импорт приложение (recipeLoop), ход — над списком треков.
import { onMounted, ref } from 'vue'
import AppIcon from './AppIcon.vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { recipePreview } from '../recipe.js'

const emit = defineEmits(['close', 'started'])
const { t } = useI18n()

const text = ref('')
const preview = ref(null)
const mode = ref('seed')    // seed — тем же сидом; plan — по плану нот (модель другая)
const busy = ref(false)
const err = ref('')

async function pick() {
  err.value = ''
  try {
    const s = await api.pickRecipe()
    if (!s) { if (!preview.value) emit('close'); return }
    let model = ''
    try { model = (await api.workerVersion())?.version?.model || '' } catch { /* воркер недоступен — без сверки модели */ }
    const p = recipePreview(s, model)
    if (p.error) { err.value = p.error; preview.value = null; return }
    text.value = s
    preview.value = p
    mode.value = p.modelDiffers ? 'plan' : 'seed'
  } catch (e) {
    err.value = String(e)
  }
}

async function start() {
  busy.value = true
  err.value = ''
  try {
    await api.importRecipe(text.value, mode.value)
    emit('started')
    emit('close')
  } catch (e) {
    err.value = String(e)
  } finally {
    busy.value = false
  }
}

onMounted(pick)
</script>

<template>
  <div class="modal-backdrop page-backdrop">
    <section class="panel page-modal">
      <div class="page-modal-head">
        <h2>{{ t('recipe.import.head') }}</h2>
        <button class="ghost icon" :title="t('common.close')" @click="emit('close')"><AppIcon name="x" /></button>
      </div>
      <div class="page-modal-body">
        <p class="muted">{{ t('recipe.import.desc') }}</p>
        <template v-if="preview">
          <p><b>{{ preview.title }}</b> · {{ t('recipe.import.steps', { n: preview.steps }) }}</p>
          <p v-if="preview.model" class="muted">{{ t('recipe.import.model', { m: preview.model }) }}</p>
          <div v-if="preview.modelDiffers" class="set-hint">
            {{ t('recipe.import.modelDiffers') }}
            <label class="set-row set-check"><input type="radio" value="seed" v-model="mode" /> {{ t('recipe.import.mode.seed') }}</label>
            <label class="set-row set-check"><input type="radio" value="plan" v-model="mode" /> {{ t('recipe.import.mode.plan') }}</label>
          </div>
          <ul v-if="preview.warnings.length" class="muted">
            <li v-for="(w, i) in preview.warnings" :key="i">{{ w }}</li>
          </ul>
        </template>
        <div v-if="err" class="error">{{ err }}</div>
        <div class="set-actions">
          <button class="ghost" @click="pick">{{ t('recipe.import.pick') }}</button>
          <button :disabled="!preview || busy" @click="start">{{ t('recipe.import.go') }}</button>
        </div>
      </div>
    </section>
  </div>
</template>
