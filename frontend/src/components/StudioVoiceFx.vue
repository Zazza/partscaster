<script setup>
// Строка «Обработка:» в блоке «Голос трека»: выбор эффекта, уместного на голосе (мегафон, телефон,
// перегруз, слэпбэк + общие с пометкой for_voice), и его ручки. Только выбор: слушать — общая кнопка
// «прослушать кусок» блока, применять — его «Применить» (эффект ляжет на дорожку «голос»).
// v-model — {chain, params} выбранного эффекта или null.
import { computed } from 'vue'
import { useI18n } from '../i18n/index.js'
import { chainDefaults, voiceFxChains } from '../dspVoice.js'

const props = defineProps({
  chains: { type: Array, default: () => [] },
})
const fxSel = defineModel({ type: Object, default: null })
const { t } = useI18n()

const fx = computed(() => voiceFxChains(props.chains))
const cur = computed(() => (fxSel.value && fx.value.find((c) => c.id === fxSel.value.chain)) || null)

// клик по эффекту — выбран (подсвечен), ручки по умолчанию; повторный клик снимает выбор
function pick(c) {
  fxSel.value = fxSel.value && fxSel.value.chain === c.id ? null : { chain: c.id, params: chainDefaults(c) }
}
function setParam(id, v) {
  fxSel.value = { ...fxSel.value, params: { ...fxSel.value.params, [id]: Number(v) } }
}
</script>

<template>
  <div class="voice-fx">
    <div class="voice-presets">
      <span class="muted" :title="t('studio.voicefx.sub')">{{ t('studio.voicefx.title') }}</span>
      <button v-for="c in fx" :key="c.id" class="toggle small-btn" :class="{ on: cur && cur.id === c.id }"
              :title="c.note" @click="pick(c)">{{ c.name }}</button>
    </div>
    <template v-if="cur">
      <p class="muted dsp-note">{{ cur.note }}</p>
      <div class="dsp-params">
        <label v-for="p in cur.params" :key="p.id">
          <span class="dsp-plabel">{{ p.label }}</span>
          <input :value="fxSel.params[p.id]" type="range" :min="p.min" :max="p.max" :step="p.step"
                 @input="setParam(p.id, $event.target.value)" />
          <span class="dsp-pval">{{ fxSel.params[p.id] }}</span>
        </label>
      </div>
    </template>
  </div>
</template>

<style scoped>
.voice-fx { margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--border); }
</style>
