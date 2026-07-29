export function normalizeEventSequence(value: unknown): number {
  const sequence = typeof value === "number"
    ? value
    : typeof value === "string" && /^[0-9]+$/.test(value)
      ? Number(value)
      : Number.NaN
  if (!Number.isSafeInteger(sequence) || sequence < 0) throw new Error("ai.event_sequence_invalid")
  return sequence
}
