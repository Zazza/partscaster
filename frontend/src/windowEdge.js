// Край растягивания окна без системной рамки. Зону у краёв (6 px) отслеживает рантайм
// Wails v2 и отдаёт растягивание оконному менеджеру; здесь — только поправка его выбора:
// верх окна — наша полоса заголовка, за него не растягиваем (угол сверху — только вбок),
// а развёрнутое окно не растягивается вовсе.
const TOP_FIX = { 'n-resize': undefined, 'nw-resize': 'w-resize', 'ne-resize': 'e-resize' }

export function frameEdge(edge, maximised) {
  if (!edge || maximised) return undefined
  return edge in TOP_FIX ? TOP_FIX[edge] : edge
}
