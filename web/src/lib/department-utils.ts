import type { Department, DepartmentTree } from "./types"

export function flattenDeptTree(tree: DepartmentTree[]): { dept: Department; depth: number }[] {
  const result: { dept: Department; depth: number }[] = []
  const stack: { node: DepartmentTree; depth: number }[] = []
  for (let i = tree.length - 1; i >= 0; i--) {
    stack.push({ node: tree[i], depth: 0 })
  }
  while (stack.length > 0) {
    const { node, depth } = stack.pop()!
    const { children: _, ...dept } = node
    result.push({ dept, depth })
    for (let i = node.children.length - 1; i >= 0; i--) {
      stack.push({ node: node.children[i], depth: depth + 1 })
    }
  }
  return result
}
