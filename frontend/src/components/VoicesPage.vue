<script setup>
// Страница «Голоса» (примерочная): ручки характера → дескриптор вокала,
// короткое draft-прослушивание на нейтральном стенде, карточки голосов.
// Карточка = ручки + seed прослушивания; аудио копируется на воркере,
// так что карточка переживает удаление исходной джобы из очереди.
import PageView from './PageView.vue'
import PageBox from './PageBox.vue'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from '../i18n/index.js'
const { t } = useI18n()
import { api } from '../api.js'
import { usePlayer } from '../composables/usePlayer.js'
import { useConfirm } from '../composables/useConfirm.js'
import VoiceKnobs from './VoiceKnobs.vue'
import {
  PARAMS_VERSION, normalizeVoiceParams, voiceDescriptor, voiceCardParams,
  auditionStyle, auditionJob, defaultVoiceParams,
} from '../voiceLab.js'

const emit = defineEmits(['close'])
const { askConfirm } = useConfirm()
const { toggleArtifact, playBtn } = usePlayer()

const params = ref(defaultVoiceParams())
const seedInput = ref('')          // пусто → новый случайный seed на каждое прослушивание
const usedSeed = ref(0)
const auditionId = ref(null)
const audition = ref(null)
const busy = ref(false)
const err = ref('')
const name = ref('')
const voices = ref([])
const autoplayedId = ref(0)      // какая проба уже включалась сама (не повторять на ре-полле)

let pollTimer = null
const descriptor = computed(() => voiceDescriptor(params.value))

async function loadVoices() {
  try { voices.value = (await api.voices()) || [] } catch {}
}

onMounted(loadVoices)
const onKey = (e) => { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
onBeforeUnmount(stopPoll)

function stopPoll() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

async function poll() {
  if (!auditionId.value) return
  try {
    const jobs = await api.jobs()
    const j = (jobs || []).find(x => x.id === auditionId.value)
    if (!j) return
    audition.value = j
    if (j.status === 'done') {
      stopPoll()
      // проба готова — включаем сами: «прослушать» и должно звучать само
      if (autoplayedId.value !== auditionId.value) {
        autoplayedId.value = auditionId.value
        await toggleArtifact('audition', t('voicelab.audition.jobTitle'),
          () => api.playAudio(auditionId.value))
      }
    } else if (['error', 'canceled'].includes(j.status)) stopPoll()
  } catch {}
}

// Прослушивание: seed всегда конкретный (воркер не возвращает использованный
// случайный seed — иначе голос не воспроизвести повторно).
async function runAudition() {
  busy.value = true
  err.value = ''
  try {
    let seed = Number.parseInt(seedInput.value, 10)
    if (!Number.isFinite(seed) || seed <= 0) seed = Math.floor(Math.random() * 2 ** 31)
    usedSeed.value = seed
    seedInput.value = String(seed)
    // свои слова уходят как есть: кириллица не переводится, под дескриптором — предупреждение
    const id = await api.submit({ ...auditionJob(params.value, seed), style: auditionStyle(descriptor.value) })
    auditionId.value = id
    audition.value = null
    stopPoll()
    pollTimer = setInterval(poll, 3000)
    poll()
  } catch (e) { err.value = String(e) } finally { busy.value = false }
}

const canSave = computed(() =>
  name.value.trim() && audition.value && audition.value.status === 'done')

async function save() {
  if (!canSave.value) return
  busy.value = true
  err.value = ''
  try {
    await api.voiceCreate(name.value.trim(), auditionId.value,
      JSON.stringify({ v: PARAMS_VERSION, ...normalizeVoiceParams(params.value) }), usedSeed.value)
    name.value = ''
    await loadVoices()
  } catch (e) { err.value = String(e) } finally { busy.value = false }
}

const cardParams = voiceCardParams
const cardDescriptor = (v) => voiceDescriptor(cardParams(v))
const cardPlay = (v) => toggleArtifact('v' + v.id, v.name, () => api.playAudio(v.job_id))

// «Переспросить»: ручки и seed из карточки — тот же голос заново.
function reaudition(v) {
  params.value = cardParams(v)
  seedInput.value = String(v.seed || '')
  runAudition()
}

function del(v) {
  askConfirm(t('voicelab.delete.title', { name: v.name }), t('voicelab.delete.body'),
    async () => {
      try { await api.voiceDelete(v.id) } catch (e) { alert(String(e)) }
      loadVoices()
    })
}

// разовая уборка: черновики прослушивания засоряют список треков.
// Карточки не трогаем — их аудио-копии хранятся отдельно от джоб.
const cleaning = ref(false)

function cleanProbes() {
  askConfirm(t('voicelab.clean.title'), t('voicelab.clean.body'), async () => {
    cleaning.value = true
    try {
      const jobs = await api.jobs()
      const probes = (jobs || []).filter((x) => x.draft && x.title === 'voice audition')
      for (const p of probes) {
        try { await api.deleteJob(p.id) } catch { /* уже нет — идём дальше */ }
      }
      if (probes.some((p) => p.id === auditionId.value)) {
        auditionId.value = null
        audition.value = null
        stopPoll()
      }
    } catch (e) { err.value = String(e) } finally { cleaning.value = false }
  })
}
</script>

<template>
  <PageView :title="t('nav.voices')">
    <template #lead>{{ t('voicelab.desc') }}</template>
    <!-- слева — характер голоса, справа — прослушать и сохранить (прилипает), ниже — карточки -->
    <div class="page-cols">
    <div class="page-main">
    <PageBox :title="'1 · ' + t('voicelab.step.character')" :hint="t('voicelab.step.character.sub')">
      <VoiceKnobs v-model="params">
        <label>
          <span :title="t('voicelab.seed.tip')">{{ t('voicelab.seed') }}</span>
          <input v-model="seedInput" :placeholder="t('voicelab.seed.tip')" inputmode="numeric" />
          <span></span>
        </label>
      </VoiceKnobs>
    </PageBox>
    </div>
    <aside class="page-side">
    <PageBox :title="'2 · ' + t('voicelab.step.listen')">
      <div class="corpus-actions">
        <button class="primary" :disabled="busy" :title="t('voicelab.audition.tip')" @click="runAudition">
          <template v-if="busy">{{ t('voicelab.audition.busy') }}</template><template v-else><AppIcon name="play" /> {{ t('voicelab.audition') }}</template>
        </button>
        <template v-if="audition">
          <span v-if="['queued', 'running'].includes(audition.status)" class="muted">
            {{ t('queue.status.' + audition.status) }} · {{ t('voicelab.audition.wait') }}<template v-if="audition.stage"> · {{ audition.stage }}</template><template v-if="audition.elapsed_s"> {{ Math.round(audition.elapsed_s) }} {{ t('common.sec') }}</template>
          </span>
          <template v-if="audition.status === 'done'">
            <button class="ghost small-btn" :title="t('voicelab.audition.replay')"
                    @click="toggleArtifact('audition', t('voicelab.audition.jobTitle'), () => api.playAudio(auditionId))">
              {{ playBtn('audition') }}
            </button>
            <span class="muted">{{ t('voicelab.audition.ready') }}</span>
          </template>
          <span v-if="audition.status === 'error'" class="error">{{ audition.error }}</span>
        </template>
      </div>

      <div v-if="audition && audition.status === 'done'" class="corpus-new">
        <input v-model="name" :placeholder="t('voicelab.save.name')" @keyup.enter="save" />
        <button class="ghost" :disabled="busy || !canSave" @click="save"><AppIcon name="save" /> {{ t('voicelab.save') }}</button>
      </div>
      <p v-if="err" class="error">{{ err }}</p>
    </PageBox>
    </aside>
    </div>

    <PageBox :title="t('voicelab.cards')">
      <template #tools>
        <button class="ghost small-btn" :disabled="cleaning"
                :title="t('voicelab.clean.tip')" @click="cleanProbes"><AppIcon name="eraser" /> {{ t('voicelab.clean') }}</button>
      </template>
      <div v-for="v in voices" :key="v.id" class="corpus-item voice-card">
        <strong>{{ v.name }}</strong>
        <span class="muted voice-card-desc">{{ cardDescriptor(v) }}</span>
        <span class="muted">seed {{ v.seed || '—' }}</span>
        <span class="spacer"></span>
        <button v-if="v.job_alive" class="ghost small-btn" @click="cardPlay(v)">{{ playBtn('v' + v.id) }}</button>
        <span v-else class="muted">{{ t('voicelab.card.sourceGone') }}</span>
        <button class="ghost small-btn" @click="reaudition(v)"><AppIcon name="repeat" /> {{ t('voicelab.card.reaudition') }}</button>
        <button class="ghost small-btn" @click="del(v)"><AppIcon name="x" /></button>
      </div>
      <p v-if="!voices.length" class="muted">{{ t('voicelab.empty') }}</p>
    </PageBox>
  </PageView>
</template>
