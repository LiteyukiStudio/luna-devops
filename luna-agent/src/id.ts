import { randomBytes } from "node:crypto"

export function createId(prefix: string): string {
  return `${prefix}_${randomBytes(12).toString("hex")}`
}
