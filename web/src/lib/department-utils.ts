import type { Department, DepartmentTree } from "./types"

export function flattenDeptTree(
  tree: DepartmentTree[],
  depth = 0
): { dept: Department; depth: number }[] {
  const result: { dept: Department; depth: number }[] = []
  for (const node of tree) {
    const { children: _, ...dept } = node
    result.push({ dept, depth })
    if (node.children.length > 0) {
      result.push(...flattenDeptTree(node.children, depth + 1))
    }
  }
  return result
}
