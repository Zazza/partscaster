// Помощники тестов (в приложение не входят): шаблон .vue как дерево разбора компилятора Vue (vue/compiler-sfc) —
// проверки «что внутри чего, под каким условием» по дереву, а не регэкспами по тексту. Форматирование, порядок
// атрибутов и переносы строк на результат не влияют.
import { readFileSync } from 'node:fs'
import { parse } from 'vue/compiler-sfc'

const ELEMENT = 1
const TEXT = 2
const INTERPOLATION = 5
const ATTRIBUTE = 6
const DIRECTIVE = 7

/** Корень дерева шаблона файла (путь — от src/). */
export function templateAst(rel) {
  const src = readFileSync(new URL(`../${rel}`, import.meta.url), 'utf8')
  const { descriptor, errors } = parse(src, { filename: rel })
  if (errors.length) throw new Error(`${rel}: ${errors[0].message}`)
  return descriptor.template.ast
}

/** Все элементы дерева, для которых pred(node) истинно: [{ node, ancestors }] (ancestors — от корня). */
export function findElements(root, pred) {
  const out = []
  const walk = (n, anc) => {
    if (n.type === ELEMENT && pred(n)) out.push({ node: n, ancestors: anc })
    for (const c of n.children || []) walk(c, n.type === ELEMENT ? [...anc, n] : anc)
  }
  walk(root, [])
  return out
}

/** Выражение директивы: dir(node, 'if'), dir(node, 'for'), dir(node, 'model'), dir(node, 'bind', 'active'),
 *  dir(node, 'on', 'click'); нет — null. */
export function dir(node, name, arg = null) {
  const p = (node.props || []).find((x) => x.type === DIRECTIVE && x.name === name
    && (arg === null || (x.arg && x.arg.content === arg)))
  return p ? (p.exp ? p.exp.content : '') : null
}

/** Обычный атрибут элемента (class="…"); нет — null. */
export function attr(node, name) {
  const p = (node.props || []).find((x) => x.type === ATTRIBUTE && x.name === name)
  return p ? (p.value ? p.value.content : '') : null
}

/** Текст элемента с потомками: статический текст и выражения {{ … }} (как написано). */
export function textOf(node) {
  if (node.type === TEXT) return node.content
  if (node.type === INTERPOLATION) return `{{ ${node.content.content} }}`
  return (node.children || []).map(textOf).join('')
}

/** Условие показа элемента: v-if / v-else-if / v-show; нет — null. */
export const shownIf = (node) => dir(node, 'if') ?? dir(node, 'else-if') ?? dir(node, 'show')

/** Имена в выражении (грубо: идентификаторы). */
export const idents = (expr) => String(expr || '').match(/[A-Za-z_$][\w$]*/g) || []

/** HTML компонента при данных props (серверная отрисовка vue/server-renderer): setup и шаблон исполняются,
 *  onMounted и клики — нет. Комментарии-разметка фрагментов (<!--[-->) убраны. */
export async function renderHtml(component, props = {}) {
  const { createSSRApp, h } = await import('vue')
  const { renderToString } = await import('vue/server-renderer')
  const { default: AppIcon } = await import('../components/AppIcon.vue')
  // AppIcon — глобальный компонент приложения (main.js)
  const html = await renderToString(createSSRApp({ render: () => h(component, props) }).component('AppIcon', AppIcon))
  return html.replace(/<!--[\s\S]*?-->/g, '')
}

/** Минимальный localStorage для node-окружения тестов. */
export class MemStorage {
  constructor(init = {}) { this.m = new Map(Object.entries(init)) }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

/** Скрипт компонента без шаблона: setup исполняется внутри живого (не серверного) экземпляра-хозяина — watch,
 *  computed, onMounted/onBeforeUnmount работают, как в приложении; шаблон не рисуется. Возвращает привязки setup
 *  (в тестах SFC собирается в режиме разработки: <script setup> отдаёт все свои имена), вызовы emit и unmount().
 *  Внешнее (api, плеер) тест подменяет vi.mock. */
export async function mountSetup(component, props = {}) {
  const { createRenderer, h, nextTick, reactive, ssrContextKey } = await import('vue')
  const node = () => ({})
  const { createApp } = createRenderer({
    insert() {}, remove() {}, createElement: node, createText: node, createComment: node, setText() {},
    setElementText() {}, parentNode: () => null, nextSibling: () => null, patchProp() {},
  })
  const emitted = []
  const state = reactive({ ...props })
  let bindings = null
  const Host = {
    setup() {
      const ctx = { expose() {}, emit: (name, ...args) => emitted.push([name, ...args]), attrs: {}, slots: {} }
      bindings = component.setup(state, ctx)
      return () => h('div')
    },
  }
  // SFC в тестах собран для сервера: модуль отмечается в контексте отрисовки — даём пустой
  const app = createApp(Host).provide(ssrContextKey, { modules: new Set() })
  app.mount({})
  await nextTick()
  return { b: bindings, props: state, emitted, unmount: () => app.unmount() }
}
