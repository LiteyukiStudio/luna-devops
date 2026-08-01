import { createHash } from "node:crypto"
import type { CreateTurn } from "../domain.js"

export function createTurnRequestHash(input: CreateTurn): string {
  const businessInput = { ...input }
  delete businessInput.traceContext
  return createHash("sha256").update(JSON.stringify(businessInput)).digest("hex")
}
