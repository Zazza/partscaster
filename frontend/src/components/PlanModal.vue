<script setup>
// Редактор плана: ABC-партитура до рендера (план, из джобы, из трека, из профиля).
import { useI18n } from '../i18n/index.js'

const { t } = useI18n()

defineProps({
  open: Boolean,
  busy: Boolean,
  err: String,
  info: Object, // {seed, seconds, truncated, fromJob, fromTrack}
})
const emit = defineEmits(['close', 'use', 'new-plan', 'from-track'])
const abc = defineModel('abc', { type: String, default: '' })
</script>

<template>
  <div v-if="open" class="modal-backdrop">
    <div class="modal">
      <div class="modal-head">
        <h2>{{ t('plan.title') }} <span class="muted">{{ t('plan.advanced') }}</span></h2>
        <span v-if="info" class="muted plan-meta">
          seed {{ info.seed }}<template v-if="info.seconds"> · {{ info.seconds }}s</template><template v-if="info.fromJob"> · {{ t('plan.fromJob', { id: info.fromJob }) }}</template><template v-if="info.truncated"> · {{ t('plan.truncated') }}</template>
        </span>
        <span class="spacer"></span>
        <button class="ghost" @click="emit('close')"><AppIcon name="x" /></button>
      </div>
      <p v-if="busy" class="muted">{{ t('plan.busy') }}</p>
      <p v-if="err" class="error">{{ err }}</p>
      <p v-if="info && info.marks && info.marks.length" class="plan-marks">
        {{ t('plan.marks') }}: {{ info.marks.map((m) => m.label).join(' · ') }}
        <span class="muted">{{ t('plan.marks.hint') }}</span>
      </p>
      <details class="abc-help">
        <summary>{{ t('plan.help') }}</summary>
        <p class="muted">
          {{ t('plan.help.syntax') }}<br />
          {{ t('plan.help.note') }}
        </p>
      </details>
      <textarea v-model="abc" rows="18" class="abc" spellcheck="false"></textarea>
      <div class="modal-actions">
        <button class="primary" :disabled="!abc.trim()" :title="t('plan.use.tip')"
                @click="emit('use', abc)">{{ t('plan.use') }}</button>
        <button class="ghost" :disabled="busy" @click="emit('new-plan')">{{ t('plan.new') }}</button>
        <button class="ghost" :disabled="busy" @click="emit('from-track')" :title="t('plan.fromTrack.tip')">{{ t('plan.fromTrack') }}</button>
      </div>
    </div>
  </div>
</template>
