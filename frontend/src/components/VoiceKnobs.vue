<script setup>
// Ручки характера голоса — общие для примерочной («Голоса») и шага «Голос» студии:
// пресеты, «в духе», регистр, слайдеры, «своё», строка дескриптора. Свои поля
// страницы (seed примерочной) — в слоте внутри сетки ручек.
import { computed } from 'vue'
import { useI18n } from '../i18n/index.js'
import VSelect from '../VSelect.vue'
import { VOICE_REGISTERS, VOICE_KNOBS, VOICE_PRESETS, VOICE_ARCHETYPES, normalizeVoiceParams, voiceDescriptor } from '../voiceLab.js'
import { hasCyrillic } from '../requests.js'

const { t } = useI18n()
const params = defineModel({ type: Object, required: true })

const descriptor = computed(() => voiceDescriptor(params.value))
const registerOptions = computed(() => VOICE_REGISTERS.map(r => ({ value: r, label: t('voicelab.register.' + r) })))

// пресет задаёт стартовые ручки — дальше пользователь докручивает руками
function applyPreset(p) {
  params.value = normalizeVoiceParams(p.params)
}
</script>

<template>
  <div class="voice-presets">
    <span class="muted">{{ t('voicelab.preset.label') }}</span>
    <button v-for="p in VOICE_PRESETS" :key="p.id" class="ghost small-btn"
            :title="voiceDescriptor(p.params)" @click="applyPreset(p)">
      <AppIcon v-if="p.id === 'reset'" name="x" /> {{ t('voicelab.preset.' + p.id) }}
    </button>
  </div>
  <div class="voice-presets">
    <span class="muted">{{ t('voicelab.archetype.label') }}</span>
    <button v-for="a in VOICE_ARCHETYPES" :key="a.id" class="ghost small-btn"
            :title="voiceDescriptor(a.params)" @click="applyPreset(a)">
      {{ t('voicelab.archetype.' + a.id) }}
    </button>
  </div>

  <div class="dsp-params">
    <label>
      <span>{{ t('voicelab.knob.register') }}</span>
      <VSelect v-model="params.register" :options="registerOptions" />
      <span></span>
    </label>
    <label v-for="k in VOICE_KNOBS" :key="k.key">
      <span>{{ t('voicelab.knob.' + k.key) }}</span>
      <input v-model.number="params[k.key]" type="range" min="0" :max="k.max" step="1" />
      <span class="dsp-pval">{{ t('voicelab.' + k.key + '.' + params[k.key]) }}</span>
    </label>
    <label>
      <span :title="t('voicelab.extra.tip')">{{ t('voicelab.extra') }}</span>
      <input v-model="params.extra" :placeholder="t('voicelab.extra.ph')" />
      <span></span>
    </label>
    <slot />
  </div>
  <p class="muted voice-hint">{{ t('voicelab.extra.hint') }}</p>
  <p class="muted voice-preview" :title="t('voicelab.preview')">{{ descriptor }}</p>
  <p v-if="hasCyrillic(descriptor)" class="voice-cyr">{{ t('form.style.cyr') }}</p>
</template>
