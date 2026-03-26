import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function sortVersionsDescending(versions: string[]): string[] {
  return [...versions].sort((a, b) => {
    const na = parseInt(a.replace("v", ""), 10)
    const nb = parseInt(b.replace("v", ""), 10)
    return nb - na
  })
}
