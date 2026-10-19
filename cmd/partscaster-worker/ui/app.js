// Окно установки воркера: шаги и ход установки. Вся логика установки — в Go (Installer, workerexe.RunInstall);
// здесь только показ: методы — window.go.main.Installer, ход — события stage/line/failed/done.
'use strict'

const api = () => window.go.main.Installer
const $ = (id) => document.getElementById(id)
const STAGES = ['Python и пакеты', 'Разделение на дорожки', 'Модель генерации', 'Запуск']
const LOG_MAX = 3000 // строк журнала в окне (полный — в файле)

let info = null
let steps = []
let at = 0
let choice = null
let phase = 'choose' // choose | installing | failed | done

function show(name) {
  for (const s of document.querySelectorAll('.step')) s.hidden = s.id !== 's-' + name
}

function setError(text) {
  $('err').textContent = text || ''
  $('err').hidden = !text
}

function buttons() {
  const last = at === steps.length - 1
  $('back').hidden = phase !== 'choose' || at === 0
  $('retry').hidden = phase !== 'failed'
  $('openlog').hidden = phase !== 'failed'
  const next = $('next')
  next.hidden = phase === 'installing' || phase === 'failed'
  next.textContent = phase === 'done' || info.gpu_error ? 'Закрыть' : last ? 'Установить' : 'Далее'
}

function gb(v) {
  return v.toFixed(1).replace('.', ',')
}

async function refreshFree() {
  const free = await api().FreeGB($('home').value)
  const el = $('free')
  if (free < 0) {
    el.textContent = `Нужно около ${info.need_gb} ГБ; свободное место узнать не удалось.`
    el.className = 'hint'
    return
  }
  el.textContent = `Свободно ${Math.floor(free)} ГБ, нужно около ${info.need_gb} ГБ.`
  el.className = free < info.need_gb ? 'hint warn' : 'hint'
  if (free < info.need_gb) el.textContent += ' Места может не хватить.'
}

function renderModels() {
  const box = $('models')
  box.innerHTML = ''
  for (const m of info.models) {
    const label = document.createElement('label')
    label.className = 'opt'
    const size = m.size_gb ? ` — файл ${gb(m.size_gb)} ГБ` : ''
    label.innerHTML = `<input type="radio" name="model" value="${m.name}"> <b>${m.name.toUpperCase()}</b>` +
      `<span>${size}</span>` + (m.recommended ? ' <span class="rec">рекомендуется для этой видеокарты</span>' : '')
    const input = label.querySelector('input')
    input.checked = m.name === info.model
    input.disabled = info.model_fixed
    box.appendChild(label)
  }
  $('low').checked = info.low_vram
  for (const r of document.querySelectorAll('input[name=lan]')) {
    r.checked = (r.value === '1') === info.lan
    r.disabled = info.lan_fixed
  }
}

function readChoice() {
  const m = document.querySelector('input[name=model]:checked')
  const lan = document.querySelector('input[name=lan]:checked')
  return {
    home: $('home').value.trim(),
    model: m ? m.value : info.model,
    low_vram: $('low').checked,
    lan: lan ? lan.value === '1' : info.lan,
  }
}

function renderStages() {
  $('stages').innerHTML = STAGES.map((s) => `<li>${s}</li>`).join('')
}

function markStage(n, cls) {
  const items = $('stages').children
  for (let i = 0; i < items.length; i++) {
    items[i].className = i < n ? 'ok' : i === n ? cls : ''
  }
}

let curStage = 0
function logLine(text) {
  const pre = $('log')
  pre.textContent += text + '\n'
  const lines = pre.textContent.split('\n')
  if (lines.length > LOG_MAX) pre.textContent = lines.slice(-LOG_MAX).join('\n')
  if (!pre.hidden) pre.scrollTop = pre.scrollHeight
  if (text.trim()) $('now').textContent = text
}

async function start() {
  setError('')
  // экран установки — до вызова: события этапов и быстрый сбой могут прийти раньше ответа Start
  const prev = phase
  phase = 'installing'
  $('fail').hidden = true
  $('inst-title').textContent = 'Установка'
  renderStages()
  curStage = 0
  markStage(0, 'cur')
  show('install')
  buttons()
  const err = await api().Start(choice)
  if (err) {
    // установка не началась (папка занята, нет места…): вернуться к выбору с текстом
    phase = prev === 'failed' ? 'failed' : 'choose'
    if (phase === 'choose' && steps.length) show(steps[at])
    setError(err)
    buttons()
  }
}

window.runtime.EventsOn('stage', (n) => {
  curStage = n
  markStage(n, 'cur')
  if (n < STAGES.length) $('now').textContent = STAGES[n] + '…'
})
window.runtime.EventsOn('line', logLine)
window.runtime.EventsOn('failed', (text) => {
  phase = 'failed'
  markStage(curStage, 'bad')
  $('inst-title').textContent = 'Установка не закончилась'
  $('fail-text').textContent = text
  $('fail').hidden = false
  buttons()
})
window.runtime.EventsOn('done', (addr) => {
  phase = 'done'
  markStage(STAGES.length, '')
  $('addr').textContent = addr
  show('done')
  buttons()
})

$('next').addEventListener('click', async () => {
  if (phase === 'done' || info.gpu_error) {
    api().Close()
    return
  }
  if (steps[at] === 'home' && !$('home').value.trim()) {
    setError('Выберите папку установки')
    return
  }
  setError('')
  if (at < steps.length - 1) {
    at++
    show(steps[at])
    buttons()
    return
  }
  choice = readChoice()
  await start()
})
$('back').addEventListener('click', () => {
  if (at > 0) at--
  setError('')
  show(steps[at])
  buttons()
})
$('retry').addEventListener('click', start)
$('openlog').addEventListener('click', () => api().OpenLog())
$('browse').addEventListener('click', async () => {
  $('home').value = await api().ChooseFolder($('home').value)
  refreshFree()
})
$('home').addEventListener('change', refreshFree)
$('copy').addEventListener('click', () => {
  api().Copy($('addr').textContent)
  $('copy').textContent = 'Скопировано'
})
$('toggle-log').addEventListener('click', () => {
  const pre = $('log')
  pre.hidden = !pre.hidden
  $('toggle-log').textContent = pre.hidden ? 'Подробнее ▾' : 'Скрыть подробности ▴'
  if (!pre.hidden) pre.scrollTop = pre.scrollHeight
})

async function init() {
  info = await api().Info()
  $('ver').textContent = info.version
  const gpu = $('gpu')
  if (info.gpu_error) {
    gpu.textContent = info.gpu_error
    gpu.className = 'card bad'
  } else {
    gpu.textContent = `Видеокарта: ${info.gpu}, ${info.vram_gb} ГБ видеопамяти.`
  }
  $('home').value = info.home
  renderModels()
  steps = ['welcome']
  if (info.ask_home) steps.push('home')
  if (info.ask_choices) steps.push('choices')
  if (!info.ask_home && !info.ask_choices && !info.gpu_error) {
    // обновление: выбор уже сделан — сразу установка недостающего
    choice = readChoice()
    await start()
    return
  }
  at = 0
  show('welcome')
  buttons()
  if (info.ask_home) refreshFree()
}

init()
