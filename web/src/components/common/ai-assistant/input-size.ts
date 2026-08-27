const utf8Encoder = new TextEncoder()

export function isAIInputWithinLimit(input: string, maxBytes?: number): boolean {
  return maxBytes === undefined || utf8Encoder.encode(input).byteLength <= maxBytes
}
