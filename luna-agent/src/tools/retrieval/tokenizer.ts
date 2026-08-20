export interface LexicalTokenizer {
  tokenize(input: string): string[]
}

/**
 * 统一处理中英文自然语言、operationId、参数名和稳定错误码。
 * 这里只做通用 Unicode/标识符切分，不根据业务关键词推断工具分类。
 */
export class UnicodeLexicalTokenizer implements LexicalTokenizer {
  private readonly segmenter = new Intl.Segmenter("zh-CN", { granularity: "word" })

  tokenize(input: string): string[] {
    const normalized = input.normalize("NFKC")
    const tokens: string[] = []

    for (const part of this.segmenter.segment(normalized)) {
      if (!part.isWordLike) continue
      pushToken(tokens, part.segment)
      for (const identifierPart of splitIdentifier(part.segment)) pushToken(tokens, identifierPart)
    }

    for (const identifier of compositeIdentifiers(normalized)) {
      pushToken(tokens, identifier)
      for (const identifierPart of splitIdentifier(identifier)) pushToken(tokens, identifierPart)
    }

    return tokens
  }
}

function pushToken(output: string[], value: string): void {
  const normalized = value.normalize("NFKC").toLocaleLowerCase("zh-CN").trim()
  if (normalized) output.push(normalized)
}

function compositeIdentifiers(input: string): string[] {
  const identifiers: string[] = []
  let current = ""
  let hasSeparator = false

  const flush = () => {
    if (current && hasSeparator) identifiers.push(current)
    current = ""
    hasSeparator = false
  }

  for (const character of input) {
    if (isLetterOrNumber(character)) {
      current += character
      continue
    }
    if (isIdentifierSeparator(character) && current) {
      current += character
      hasSeparator = true
      continue
    }
    flush()
  }
  flush()
  return identifiers
}

function splitIdentifier(input: string): string[] {
  const parts: string[] = []
  let current = ""
  let previousKind: CharacterKind | undefined

  const flush = () => {
    if (current) parts.push(current)
    current = ""
    previousKind = undefined
  }

  for (const character of input) {
    if (isIdentifierSeparator(character)) {
      flush()
      continue
    }
    if (!isLetterOrNumber(character)) {
      flush()
      continue
    }

    const kind = characterKind(character)
    if (current && shouldSplit(previousKind, kind)) flush()
    current += character
    previousKind = kind
  }
  flush()
  return parts
}

type CharacterKind = "upper" | "lower" | "number" | "other"

function characterKind(character: string): CharacterKind {
  if (isNumber(character)) return "number"
  if (character.toLocaleUpperCase("en-US") === character && character.toLocaleLowerCase("en-US") !== character) return "upper"
  if (character.toLocaleLowerCase("en-US") === character && character.toLocaleUpperCase("en-US") !== character) return "lower"
  return "other"
}

function shouldSplit(previous: CharacterKind | undefined, current: CharacterKind): boolean {
  if (!previous || previous === "other" || current === "other") return false
  if (previous === "lower" && current === "upper") return true
  return previous === "number" !== (current === "number")
}

function isIdentifierSeparator(character: string): boolean {
  return character === "." || character === "_" || character === "-" || character === "/" || character === ":"
}

function isLetterOrNumber(character: string): boolean {
  return /^(?:\p{L}|\p{N})$/u.test(character)
}

function isNumber(character: string): boolean {
  return /^\p{N}$/u.test(character)
}
