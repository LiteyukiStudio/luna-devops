/**
 * 常见 AI 模型的平台建议价格与能力目录。
 *
 * 换算口径：1 美元 = 100 平台 Credits。表中数值 = 官方美元刊例价（每 100 万
 * Token）× 100，即每 100 万 Token 的建议 Credits；缓存价格仅在官方单独公布
 * 缓存刊例价时填写，否则为 `0`，表示不单独对缓存 Token 计费。
 *
 * 上下文与输出上限取各厂商官方模型文档的标称值，并按平台表单上限
 * （上下文 2097152 / 输出 262144 Token）收敛。价格与能力来源均为各厂商官方
 * 文档，仅作为管理员填写模型目录时的参考默认值，管理员可以任意修改；
 * 未收录的模型不展示建议。
 *
 * 数据时效：2026-08 与各厂商官方定价页、new-api（QuantumNous/new-api）比率表
 * 及 OpenRouter 实时模型目录交叉核对。新增模型时按同一口径补充，价格与能力
 * 优先采用官方一手资料；转售渠道（OpenRouter 等）仅用于确认型号存在与量级。
 */
export interface SuggestedModelPrice {
  input: string
  output: string
  cachedInput: string
  cachedOutput: string
}

export interface SuggestedModelPreset {
  /** 供下拉展示与回填的规范模型名（目录 matches 中的第一个值）。 */
  displayName: string
  prices: SuggestedModelPrice
  maxContextTokens: number
  maxOutputTokens: number
}

interface SuggestedModelEntry {
  /** 归一化（小写）后的精确匹配名。 */
  matches: string[]
  /** 归一化后以该前缀开头的模型名（用于带日期或版本后缀的型号）。 */
  prefixes?: string[]
  prices: SuggestedModelPrice
  /** 官方标称上下文窗口。 */
  maxContextTokens: number
  /** 官方标称单次最大输出。 */
  maxOutputTokens: number
}

function entry(
  matches: string[],
  prices: [string, string, string?, string?],
  capability: [context: number, output: number],
  prefixes?: string[],
): SuggestedModelEntry {
  return {
    matches,
    prefixes,
    prices: {
      input: prices[0],
      output: prices[1],
      cachedInput: prices[2] ?? '0',
      cachedOutput: prices[3] ?? '0',
    },
    maxContextTokens: capability[0],
    maxOutputTokens: capability[1],
  }
}

const suggestedModelPresets: SuggestedModelEntry[] = [
  // OpenAI（https://openai.com/api/pricing/；能力 https://platform.openai.com/docs/models，
  // 并与 new-api @2026-08、OpenRouter 实时目录交叉核对）
  entry(['gpt-5.6-sol', 'gpt-5.6-sol-pro'], ['250', '1500', '25'], [1050000, 128000]),
  entry(['gpt-5.6-terra', 'gpt-5.6-terra-pro'], ['200', '1200', '20'], [1050000, 128000]),
  entry(['gpt-5.6-luna', 'gpt-5.6-luna-pro'], ['20', '120', '2'], [1050000, 128000]),
  entry(['gpt-5.5'], ['500', '3000', '50'], [1050000, 128000], ['gpt-5.5-']),
  entry(['gpt-5.5-pro'], ['3000', '18000'], [1050000, 128000]),
  entry(['gpt-5.4'], ['250', '1500', '25'], [1050000, 128000], ['gpt-5.4-2']),
  entry(['gpt-5.4-mini'], ['75', '450', '7.5'], [400000, 128000], ['gpt-5.4-mini-']),
  entry(['gpt-5.4-nano'], ['20', '125', '2'], [400000, 128000], ['gpt-5.4-nano-']),
  entry(['gpt-5.4-pro'], ['3000', '18000'], [1050000, 128000]),
  entry(['gpt-5.3-codex'], ['175', '1400', '17.5'], [400000, 128000], ['gpt-5.3-codex-']),
  entry(['gpt-5.2'], ['175', '1400', '17.5'], [400000, 128000], ['gpt-5.2-']),
  entry(['gpt-5.2-pro'], ['2100', '16800'], [400000, 128000]),
  entry(['gpt-5.1'], ['125', '1000', '12.5'], [400000, 128000], ['gpt-5.1-']),
  entry(['gpt-5.1-codex-max'], ['125', '1000', '12.5'], [400000, 128000]),
  entry(['gpt-5'], ['125', '1000', '12.5'], [400000, 128000], ['gpt-5-2']),
  entry(['gpt-5-mini'], ['25', '200', '2.5'], [400000, 128000], ['gpt-5-mini-']),
  entry(['gpt-5-nano'], ['5', '40', '0.5'], [400000, 128000], ['gpt-5-nano-']),
  entry(['gpt-5-pro', 'gpt-5-pro-2025-10-06'], ['1500', '12000'], [400000, 131072]),
  entry(['gpt-4.1'], ['200', '800', '50'], [1047576, 32768], ['gpt-4.1-']),
  entry(['gpt-4.1-mini'], ['40', '160', '10'], [1047576, 32768], ['gpt-4.1-mini-']),
  entry(['gpt-4.1-nano'], ['10', '40', '2.5'], [1047576, 32768], ['gpt-4.1-nano-']),
  entry(['gpt-4o', 'gpt-4o-2024-08-06', 'gpt-4o-2024-11-20', 'gpt-4o-2024-05-13'], ['250', '1000', '125'], [128000, 16384]),
  entry(['gpt-4o-mini'], ['15', '60', '7.5'], [128000, 16384], ['gpt-4o-mini-']),
  entry(['o3', 'o3-2025-04-16'], ['200', '800', '50'], [200000, 100000]),
  entry(['o4-mini', 'o4-mini-2025-04-16'], ['110', '440', '27.5'], [200000, 100000]),

  // Anthropic Claude（https://claude.com/pricing；缓存读取为输入价 10%，缓存写入不单独计费）
  entry(['claude-opus-4.8', 'claude-opus-4-8'], ['500', '2500', '50'], [1000000, 128000], ['claude-opus-4-8-']),
  entry(['claude-opus-4.7', 'claude-opus-4-7'], ['500', '2500', '50'], [1000000, 128000], ['claude-opus-4-7-']),
  entry(['claude-opus-4.6', 'claude-opus-4-6'], ['500', '2500', '50'], [200000, 131072], ['claude-opus-4-6-']),
  entry(['claude-opus-4.5', 'claude-opus-4-5'], ['500', '2500', '50'], [200000, 64000], ['claude-opus-4-5-']),
  entry(['claude-opus-4.1', 'claude-opus-4-1'], ['1500', '7500', '150'], [200000, 32768], ['claude-opus-4-1-']),
  entry(['claude-opus-4', 'claude-opus-4-0'], ['1500', '7500', '150'], [200000, 32768], ['claude-opus-4-']),
  entry(['claude-sonnet-4.6', 'claude-sonnet-4-6'], ['300', '1500', '30'], [1000000, 128000], ['claude-sonnet-4-6-']),
  entry(['claude-sonnet-4.5', 'claude-sonnet-4-5'], ['300', '1500', '30'], [1000000, 64000], ['claude-sonnet-4-5-']),
  entry(['claude-sonnet-4', 'claude-sonnet-4-0'], ['300', '1500', '30'], [1000000, 64000], ['claude-sonnet-4-20250514']),
  entry(['claude-haiku-4.5', 'claude-haiku-4-5'], ['100', '500', '10'], [200000, 64000], ['claude-haiku-4-5-']),
  entry(['claude-3.5-sonnet', 'claude-3-5-sonnet-20241022', 'claude-3-5-sonnet-20240620'], ['300', '1500', '30'], [200000, 8192]),
  entry(['claude-3.5-haiku', 'claude-3-5-haiku-20241022'], ['80', '400', '8'], [200000, 8192]),

  // Google Gemini（https://ai.google.dev/gemini-api/docs/pricing）
  entry(['gemini-3.1-pro', 'gemini-3.1-pro-preview'], ['200', '1200', '20'], [1048576, 65536]),
  entry(['gemini-3.1-flash-lite', 'gemini-3.1-flash-lite-preview'], ['25', '150', '2.5'], [1048576, 65536]),
  entry(['gemini-3-pro', 'gemini-3-pro-preview'], ['200', '1200', '20'], [1048576, 65536]),
  entry(['gemini-3-flash', 'gemini-3-flash-preview'], ['50', '300', '5'], [1048576, 65536]),
  entry(['gemini-2.5-pro'], ['125', '1000', '12.5'], [1048576, 65536]),
  entry(['gemini-2.5-flash'], ['30', '250', '3'], [1048576, 65536]),
  entry(['gemini-2.5-flash-lite'], ['10', '40', '1'], [1048576, 65536]),

  // DeepSeek（https://api-docs.deepseek.com/quick_start/pricing；缓存价即官方缓存命中价。
  // 官方 API 名 deepseek-chat / deepseek-reasoner 当前指向 V3.2 系列）
  entry(['deepseek-chat', 'deepseek-v3.2'], ['28', '42', '2.8'], [131072, 8192]),
  entry(['deepseek-reasoner'], ['55', '219', '5.5'], [131072, 65536]),

  // 阿里云百炼（https://help.aliyun.com/zh/model-studio/models）
  entry(['qwen3.8-max'], ['200', '600', '25'], [1000000, 131072]),
  entry(['qwen3.7-max'], ['148', '442', '29.5'], [1000000, 131072]),
  entry(['qwen3.7-plus'], ['32', '128', '6.4'], [1000000, 131072]),
  entry(['qwen3-max', 'qwen3-max-preview'], ['120', '600'], [262144, 65536]),
  entry(['qwen-plus', 'qwen-plus-latest'], ['80', '200'], [1000000, 32768]),
  entry(['qwen-flash'], ['15', '150'], [1000000, 32768]),
  entry(['qwen3-235b-a22b'], ['70', '280'], [262144, 32768]),
  entry(['qwen3-32b'], ['40', '80'], [131072, 16384]),

  // Moonshot Kimi（https://platform.moonshot.ai/docs/pricing/chat；缓存价即官方缓存命中价。
  // kimi-k3 为 2026-08 新型号，官方刊例价页尚未更新，价格为 OpenRouter 当下观测值（含转售溢价），
  // 能力取官方标称 1M 上下文，输出上限按 K2 系列对齐保守取值；待官方定价页更新后校准）
  entry(['kimi-k3'], ['300', '1500', '30'], [1048576, 65536], ['kimi-k3-']),
  entry(['kimi-k2.7-code'], ['95', '400', '19'], [262144, 32768]),
  entry(['kimi-k2.6'], ['56', '236', '9.4'], [262144, 32768]),
  entry(['kimi-k2.5', 'kimi-k2-5'], ['400', '2000', '70'], [262144, 32768], ['kimi-k2.5-']),
  entry(['kimi-k2-0905-preview', 'kimi-k2'], ['400', '1600', '100'], [262144, 32768]),
  entry(['kimi-k2-turbo-preview'], ['800', '3200', '200'], [262144, 32768]),
  entry(['kimi-k2-thinking', 'kimi-k2-thinking-turbo'], ['600', '2500', '100'], [262144, 32768]),

  // 字节跳动豆包 / 火山方舟（https://www.volcengine.com/docs/82379/1544106）
  entry(['doubao-seed-1.6', 'doubao-seed-1-6-250615'], ['80', '800'], [262144, 16384]),
  entry(['doubao-seed-1.6-flash', 'doubao-seed-1-6-flash-250715'], ['15', '150'], [262144, 16384]),
  entry(['doubao-seed-1.6-lite', 'doubao-seed-1-6-lite-251015'], ['30', '300'], [262144, 16384]),

  // 智谱 GLM（https://docs.bigmodel.cn/cn/guide/models）
  entry(['glm-5.3'], ['140', '440', '26'], [1048576, 131072]),
  entry(['glm-5.2'], ['97', '304', '19.3'], [1048576, 131072]),
  entry(['glm-5.1'], ['97', '304', '17.9'], [204800, 128000]),
  entry(['glm-5'], ['600', '2200'], [202752, 131072]),
  entry(['glm-4.7'], ['400', '1800'], [202752, 131072]),
  entry(['glm-4.6'], ['200', '600'], [202752, 131072]),
  entry(['glm-4.5'], ['200', '600'], [131072, 98304], ['glm-4.5-']),
  entry(['glm-4.5-flash'], ['0', '0'], [131072, 98304]),
]

function normalizeModelName(name: string): string {
  return name.trim().toLowerCase()
}

function matchPreset(normalized: string): SuggestedModelEntry | undefined {
  if (!normalized) {
    return undefined
  }
  for (const item of suggestedModelPresets) {
    if (item.matches.includes(normalized)) {
      return item
    }
  }
  for (const item of suggestedModelPresets) {
    if (item.prefixes?.some(prefix => normalized.startsWith(prefix))) {
      return item
    }
  }
  return undefined
}

function toPreset(item: SuggestedModelEntry): SuggestedModelPreset {
  return {
    displayName: item.matches[0]!,
    prices: item.prices,
    maxContextTokens: item.maxContextTokens,
    maxOutputTokens: item.maxOutputTokens,
  }
}

/**
 * 按 Provider 模型名查找建议价格。先精确匹配，再按前缀匹配带日期/版本
 * 后缀的型号；未收录时返回 `undefined`。
 */
export function findSuggestedModelPrice(name: string): SuggestedModelPrice | undefined {
  return matchPreset(normalizeModelName(name))?.prices
}

/**
 * 按 Provider 模型名查找完整建议预设（价格 + 上下文/输出上限）。
 */
export function findSuggestedModelPreset(name: string): SuggestedModelPreset | undefined {
  const item = matchPreset(normalizeModelName(name))
  return item ? toPreset(item) : undefined
}

/**
 * 全部建议预设目录，用于模型表单的可选列表。按厂商收录顺序返回。
 */
export function listSuggestedModelPresets(): SuggestedModelPreset[] {
  return suggestedModelPresets.map(toPreset)
}
