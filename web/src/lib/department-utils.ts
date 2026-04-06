import type { DepartmentTree } from "./types"

export function flattenDeptTree(
  tree: DepartmentTree[],
  depth = 0
): { dept: { id: string; name: string }; depth: number }[] {
  const result: { dept: { id: string; name: string }; depth: number }[] = []
  for (const node of tree) {
    result.push({ dept: { id: node.id, name: node.name }, depth })
    if (node.children.length > 0) {
      result.push(...flattenDeptTree(node.children, depth + 1))
    }
  }
  return result
}
