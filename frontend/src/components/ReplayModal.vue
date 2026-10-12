<script setup>
// «Переиграть в другом стиле»: YuE заново играет трек по его нотам в новом стиле — кавер
// новым треком или слой поверх оригинала. Запрос собирает replay.js (replayRequest), отправка — SubmitOverdub.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppIcon from './AppIcon.vue'
import VSelect from '../VSelect.vue'
import { useI18n } from '../i18n/index.js'
import { api } from '../api.js'
import { odPartyChips, slotOptions } from '../slotOptions.js'
import { replayRequest } from '../replay.js'

const props = defineProps({
  job: { type: Object, required: true },
  autoTranslate: { type: Boolean, default: true },   // кириллицу «своими словами» — на английский (как стили)
})
const emit = defineEmits(['close', 'submitted'])
const { t } = useI18n()

const genre = ref('')
const chips = ref(new Set())
const text = ref('')
const voice = ref('lyrics')       // lyrics | none | own
const ownLyrics = ref(props.job.lyrics || '')
const mix = ref('cover')          // cover — новый трек; layer — поверх оригинала
const gain = ref(0.5)
const close = ref(true)           // ближе к оригиналу — сид трека
const busy = ref(false)
const err = ref('')

const genreOpts = computed(() => [{ value: '', label: t('replay.genre.none') },
  ...(slotOptions.genre || []).map(([ru, en]) => ({ value: en, label: ru }))])

function toggleChip(i) {
  const s = new Set(chips.value)
  s.has(i) ? s.delete(i) : s.add(i)
  chips.value = s
}

async function submit() {
  err.value = ''
  let txt = text.value.trim()
  if (txt && props.autoTranslate && /[а-яё]/i.test(txt)) {
    try { txt = (await api.translate(txt)).text || txt } catch { /* без перевода — как ввели */ }
  }
  const req = replayRequest({ genre: genre.value, chips: [...chips.value].map((i) => odPartyChips[i].en), text: txt,
    voice: voice.value, ownLyrics: ownLyrics.value, mix: mix.value, gain: gain.value, close: close.value }, props.job)
  if (!req) { err.value = t('replay.empty'); return }
  busy.value = true
  try {
    const id = await api.submitOverdub(props.job.id, req.style, req.lyrics, req.gain, '', req.seed)
    // имя нового трека — понятное, а не «overdub of #N»
    const what = genreOpts.value.find((o) => o.value === genre.value && o.value)?.label || t('replay.title.style')
    try { await api.renameJob(id, `${props.job.title || '#' + props.job.id} · ${t('replay.title.suffix')}: ${what}`) } catch { /* имя — не главное */ }
    emit('submitted', id)
    emit('close')
  } catch (e) { err.value = String(e) } finally { busy.value = false }
}

const onKey = (e) => { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="modal-backdrop page-backdrop">
    <section class="panel page-modal replay-modal">
      <div class="page-modal-head">
        <h2>{{ t('replay.head') }} · {{ job.title || '#' + job.id }}</h2>
        <button class="ghost icon" :title="t('common.close')" @click="emit('close')"><AppIcon name="x" /></button>
      </div>
      <div class="page-modal-body">
        <p class="muted">{{ t('replay.desc') }}</p>
        <label class="replay-row">
          <span>{{ t('replay.genre') }}</span>
          <VSelect v-model="genre" :options="genreOpts" searchable />
        </label>
        <div class="replay-row">
          <span>{{ t('replay.parts') }}</span>
          <div class="od-chips">
            <button v-for="(c, ci) in odPartyChips" :key="ci" class="toggle" :class="{ on: chips.has(ci) }"
                    @click="toggleChip(ci)">{{ c.ru }}</button>
          </div>
        </div>
        <label class="replay-row">
          <span>{{ t('replay.text') }}</span>
          <input v-model="text" :placeholder="t('replay.text.ph')" />
        </label>
        <div class="replay-row">
          <span>{{ t('replay.voice') }}</span>
          <span class="replay-radios">
            <label><input v-model="voice" type="radio" value="lyrics" /> {{ t('replay.voice.lyrics') }}</label>
            <label><input v-model="voice" type="radio" value="none" /> {{ t('replay.voice.none') }}</label>
            <label><input v-model="voice" type="radio" value="own" /> {{ t('replay.voice.own') }}</label>
          </span>
        </div>
        <textarea v-if="voice === 'own'" v-model="ownLyrics" rows="5" class="od-lyrics"></textarea>
        <div class="replay-row">
          <span>{{ t('replay.mix') }}</span>
          <span class="replay-radios">
            <label :title="t('replay.mix.cover.tip')"><input v-model="mix" type="radio" value="cover" /> {{ t('replay.mix.cover') }}</label>
            <label :title="t('replay.mix.layer.tip')"><input v-model="mix" type="radio" value="layer" /> {{ t('replay.mix.layer') }}</label>
            <label v-if="mix === 'layer'" class="od-gain">{{ t('replay.gain') }}
              <input v-model.number="gain" type="range" min="0.1" max="1" step="0.05" /> {{ gain }}</label>
          </span>
        </div>
        <div class="replay-row">
          <span>{{ t('replay.close') }}</span>
          <span class="replay-radios">
            <label><input v-model="close" type="radio" :value="true" /> {{ t('replay.close.yes') }}</label>
            <label><input v-model="close" type="radio" :value="false" /> {{ t('replay.close.no') }}</label>
          </span>
        </div>
        <p v-if="err" class="err">{{ err }}</p>
        <div class="corpus-actions">
          <button class="primary" :disabled="busy" @click="submit">{{ busy ? '…' : t('replay.go') }}</button>
          <button class="ghost" @click="emit('close')">{{ t('common.cancel') }}</button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.replay-modal { width: min(820px, 96vw); }
.replay-row { display: grid; grid-template-columns: 160px minmax(0, 1fr); align-items: center; gap: 10px; margin: 6px 0; }
.replay-radios { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; }
.replay-radios label { display: inline-flex; gap: 6px; align-items: center; }
</style>
