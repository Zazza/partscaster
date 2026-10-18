// Единое состояние встроенного плеера: один инстанс на приложение
// (state в module scope — компоненты зовут usePlayer() где угодно).
import { ref, watch } from 'vue'
import { api } from '../api.js'
import { useI18n } from '../i18n/index.js'

const { t } = useI18n()

const playerState = ref({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 })
const nowPlaying = ref('')     // подпись встроенного плеера
const playBusy = ref({})
// key — уникальный идентификатор артефакта: 'm15' — трек, 's15:stem-bass' — стем
const nowPlayingKey = ref('')
// последний запущенный артефакт: «плей» панели после «стопа»/конца играет его снова,
// а не что попало (в студии — превью пресета, волна или микс; в списке — трек)
const lastArtifact = ref(null)   // {key, label, jobId, file, dur} | null
// студийный контекст панели: открытая студия трека. «Плей» без истории
// прослушивания играет сам трек, с активными правками — свежий микс
// (последняя правка); прослушанное этого трека поновее (превью, вариант)
// главнее. Вне студии играет последний запущенный артефакт
const studioCtx = ref(null)      // {jobId, file, mainFile, mixFile, dur, label, ensure?} | null; ensure — собрать микс, если устарел (→ файл)
function setStudioCtx(v) { studioCtx.value = v }
const volume = ref(parseFloat(localStorage.getItem('yue_volume') ?? '0.8'))

// рельса перемотки: тянем локально, на отпускании — команда в плеер
const seekPos = ref(0)
let seeking = false

// «играть выделенное» студии: верхний ▶ при активном выделении гоняет кусок
// [from, to] артефакта волны — один прогон, стоп в конце (rangeUntil).
const playRange = ref(null)   // {key, jobId, file, dur, from, to} | null
const rangeUntil = ref(null)  // {key, to} — идущий прогон, стоп по нему

let refreshCb = null
// App регистрирует колбэк общего обновления (джобы + плеер)
function onRefresh(fn) { refreshCb = fn }

async function refreshPlayer() {
  try { playerState.value = await api.audioState() } catch {}
  // трек доиграл до конца: плеер держит его загруженным (позиция = длительность),
  // но кнопки должны вернуться в «▶»; пауза в середине — по-прежнему «играет»
  if (playbackEnded(playerState.value)) nowPlayingKey.value = ''
}

// конец воспроизведения: не играет и позиция у самого конца (полсекунды запаса
// на округление таймера плеера)
export function playbackEnded(st) {
  return !!st && !st.playing && st.duration_sec > 0 && st.position_sec >= st.duration_sec - 0.5
}

function isPlaying(key) {
  return playerState.value.job_id > 0 && nowPlayingKey.value === key
}

async function toggleArtifact(key, label, play, artifact) {
  if (playBusy.value[key]) return
  if (isPlaying(key)) {
    await api.stopAudio()
    nowPlayingKey.value = ''
    setTimeout(() => refreshCb && refreshCb(), 200)
    return
  }
  playBusy.value = { ...playBusy.value, [key]: true }
  nowPlaying.value = label
  try {
    await play()
    nowPlayingKey.value = key
    // запомнить, что играть по «плей» панели после стопа (файл известен — иначе нечего)
    if (artifact && artifact.file) lastArtifact.value = { key, label, ...artifact }
  } catch (e) {
    nowPlaying.value = t('player.errorDetail', { err: String(e) })
  } finally {
    playBusy.value = { ...playBusy.value, [key]: false }
    setTimeout(() => refreshCb && refreshCb(), 300)
  }
}

// отметить запущенный артефакт (прямой playFile без toggleArtifact): «плей» панели
// после «стопа» вернётся именно к нему
function markNow(key, label, jobId, file, dur) {
  if (!file) return
  lastArtifact.value = { key, label, jobId, file, dur }
  nowPlaying.value = label
  nowPlayingKey.value = key
}

function playBtn(key) {
  if (playBusy.value[key]) return '…'
  return isPlaying(key) ? '■' : '▶'
}

function setPlayRange(r) {
  playRange.value = r
  if (!r) rangeUntil.value = null
}

// один прогон выделенного куска: ползунок и тайминг шапки — сразу с начала
async function playRangeOnce(r) {
  if (playBusy.value[r.key]) return
  playBusy.value = { ...playBusy.value, [r.key]: true }
  nowPlaying.value = t('player.range', { from: fmtDur(r.from), to: fmtDur(r.to) })
  try {
    await api.playFile(r.jobId, r.file, r.dur)
    nowPlayingKey.value = r.key
    seekPos.value = r.from
    await api.seekAudio(r.from)
    rangeUntil.value = { key: r.key, to: r.to }
  } catch (e) {
    nowPlaying.value = t('player.errorDetail', { err: String(e) })
  } finally {
    playBusy.value = { ...playBusy.value, [r.key]: false }
    setTimeout(() => refreshCb && refreshCb(), 300)
  }
}

// ▶ шапки: есть выделение в студии — гоняет кусок (пауза внутри куска —
// продолжение с места паузы, стоп/конец — новый прогон с начала); иначе —
// обычная пауза/продолжение загруженного. После «стопа»/конца возвращается
// последний запущенный артефакт; выделение гоняет кусок, пока оно и есть
// последний запущенный («послушать превью» + плей панели не должен уводить
// на выделение волны — играло другое)
async function togglePlay() {
  const r = playRange.value
  const a = lastArtifact.value
  if (r && (!a || a.key === r.key)) {
    const st = playerState.value
    if (nowPlayingKey.value === r.key && st.job_id === r.jobId && st.job_id > 0
        && (st.playing || (st.position_sec > r.from && st.position_sec < r.to))) {
      return api.toggleAudio()
    }
    return playRangeOnce(r)
  }
  const st = playerState.value
  // цель «плея»: последнее прослушанное трека студии (превью, вариант, микс);
  // в студии без него — сам трек, а с активными правками — свежий микс;
  // вне студии — последний запущенный артефакт
  const ctx = studioCtx.value
  let target = a && (!ctx || a.jobId === ctx.jobId) ? a : ctx
  // последним играл сам трек или текущий микс черновика (он мог устареть: ctx.ensure соберёт заново) — цель студия;
  // старая версия микса на волне и превью — как слушали
  if (ctx && a && a.jobId === ctx.jobId && ctx.mixFile && (a.file === ctx.mainFile || a.file === ctx.mixFile)) target = ctx
  // загружено то же, что целью, и не остановлено — пауза/продолжение
  if (st.job_id > 0 && (!target || target.jobId === st.job_id) && !playbackEnded(st) && st.position_sec > 0) {
    return api.toggleAudio()
  }
  // после «стопа»/конца (или с нуля) — цель заново
  if (target && target.file) {
    if (playBusy.value[target.key || target.file]) return
    const busyKey = target.key || target.file
    playBusy.value = { ...playBusy.value, [busyKey]: true }
    nowPlaying.value = target.label
    try {
      const file = (target === ctx && ctx.ensure && (await ctx.ensure())) || target.file
      await api.playFile(target.jobId, file, target.dur)
      nowPlayingKey.value = busyKey
    } catch (e) {
      nowPlaying.value = t('player.errorDetail', { err: String(e) })
    } finally {
      playBusy.value = { ...playBusy.value, [busyKey]: false }
      setTimeout(() => refreshCb && refreshCb(), 300)
    }
    return
  }
  return api.toggleAudio()
}

async function stopAll() {
  await api.stopAudio()
  nowPlayingKey.value = ''
  await refreshPlayer()
}

function onVolume() {
  localStorage.setItem('yue_volume', String(volume.value))
  api.setVolume(volume.value)
}

function onSeekInput() { seeking = true }
async function onSeekChange() {
  seeking = false
  try { await api.seekAudio(seekPos.value) } catch {}
  setTimeout(refreshPlayer, 300)
}

export function fmtDur(s) {
  if (!s) return ''
  // округляем ДО деления: floor+round(остатка) давал «1:60» вместо «2:00»
  const total = Math.round(s)
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
}

export function usePlayer() {
  return {
    playerState, nowPlaying, playBusy, nowPlayingKey, lastArtifact, studioCtx, volume, seekPos,
    refreshPlayer, isPlaying, toggleArtifact, markNow, setStudioCtx, playBtn, stopAll,
    onVolume, onSeekInput, onSeekChange, onRefresh,
    playRange, rangeUntil, setPlayRange, togglePlay,
  }
}

// инициализация один раз при первом импорте
watch(() => playerState.value.position_sec, (p) => { if (!seeking) seekPos.value = p || 0 })
setInterval(refreshPlayer, 1000)
