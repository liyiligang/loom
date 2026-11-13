import type { LoopPlan } from '../types'

// The server accepts only success DAGs. Layout never executes AI-provided code.
export function loopLayout(plan: LoopPlan) {
  const rank = new Map(plan.nodes.map(node => [node.id, 0]))
  for (let pass = 0; pass < plan.nodes.length; pass++) {
    for (const edge of plan.edges.filter(edge => edge.condition === 'success')) rank.set(edge.to, Math.max(rank.get(edge.to)!, rank.get(edge.from)! + 1))
  }
  const rows = new Map<number, number>()
  const nodes = plan.nodes.map(node => {
    const column = rank.get(node.id)!
    const row = rows.get(column) ?? 0
    rows.set(column, row + 1)
    return { ...node, x: 44 + column * 292, y: 92 + row * 184, rank: column }
  })
  const index = new Map(nodes.map(node => [node.id, node]))
  const edges = plan.edges.map(edge => {
    const from = index.get(edge.from)!, to = index.get(edge.to)!
    const x1 = from.x + 224, y1 = from.y + 71, x2 = to.x, y2 = to.y + 71
    const bend = edge.condition === 'failure' ? `M${from.x + 112},${from.y} C${from.x + 112},16 ${to.x + 112},16 ${to.x + 112},${to.y}` : `M${x1},${y1} C${x1 + 38},${y1} ${x2 - 38},${y2} ${x2},${y2}`
    return { ...edge, path: bend, startX: x1, startY: y1, endX: x2, endY: y2 }
  })
  return { nodes, edges, width: Math.max(...nodes.map(node => node.x)) + 280, height: Math.max(...nodes.map(node => node.y)) + 200 }
}
