export interface LexicalTokenizer {
  tokenize(input: string): string[]
}

const cjkParticleStopwords = new Set([..."的了着过吗呢吧啊呀嘛与和及或并且把被将给是于在从到对为这那之其每各某该个条项中"])
const hanCharacterPattern = /^\p{Script=Han}$/u

/**
 * 统一处理中英文自然语言、operationId、参数名和稳定错误码。
 * 中文词法处理：
 * 1. 过滤高频单字语法虚词/量词（如“的/并/在/是/为/中/个”），消除跨领域单字无差别碰撞；
 * 2. 对连续被拆碎的单字实义片段补充双字词组（如“扩+容 -> 扩容”、“拓+扑+连+线 -> 拓扑/扑连/连线”），
 *    同时保留非虚词实义字根（如“填/卷”）以兼顾短词根匹配。
 */
export class UnicodeLexicalTokenizer implements LexicalTokenizer {
  private readonly segmenter = new Intl.Segmenter("zh-CN", { granularity: "word" })

  tokenize(input: string): string[] {
    const normalized = input.normalize("NFKC")
    const tokens: string[] = []
    let pendingSingleHan: string[] = []
    let previousSingleHanEnd = -1

    const flushSingleHan = () => {
      if (pendingSingleHan.length >= 2) {
        for (let index = 0; index < pendingSingleHan.length - 1; index += 1) {
          pushToken(tokens, `${pendingSingleHan[index]}${pendingSingleHan[index + 1]}`)
        }
      }
      pendingSingleHan = []
      previousSingleHanEnd = -1
    }

    for (const part of this.segmenter.segment(normalized)) {
      if (!part.isWordLike) {
        flushSingleHan()
        continue
      }
      if (isPureHan(part.segment)) {
        const chars = [...part.segment]
        if (chars.length === 1) {
          const char = chars[0]!
          if (cjkParticleStopwords.has(char)) {
            flushSingleHan()
            continue
          }
          pushToken(tokens, char)
          if (pendingSingleHan.length > 0 && part.index !== previousSingleHanEnd) {
            flushSingleHan()
          }
          pendingSingleHan.push(char)
          previousSingleHanEnd = part.index + part.segment.length
          continue
        }
        flushSingleHan()
        pushToken(tokens, part.segment)
        continue
      }
      flushSingleHan()
      pushToken(tokens, part.segment)
      for (const identifierPart of splitIdentifier(part.segment)) pushToken(tokens, identifierPart)
    }
    flushSingleHan()

    for (const identifier of compositeIdentifiers(normalized)) {
      pushToken(tokens, identifier)
      for (const identifierPart of splitIdentifier(identifier)) pushToken(tokens, identifierPart)
    }

    return tokens
  }
}

function isPureHan(value: string): boolean {
  if (!value) return false
  for (const character of value) {
    if (!hanCharacterPattern.test(character)) return false
  }
  return true
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
