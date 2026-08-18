/**
 * 常见 AI 模型的平台建议价格目录。
 *
 * 换算口径：1 美元 = 100 平台 Credits。表中数值 = 官方美元刊例价（每 100 万
 * Token）× 100，即每 100 万 Token 的建议 Credits；缓存价格仅在官方单独公布
 * 缓存刊例价时填写，否则为 `0`，表示不单独对缓存 Token 计费。
 *
 * 价格来源为各厂商官方定价页，仅作为管理员填写模型目录时的参考默认值，
 * 管理员可以任意修改；未收录的模型不展示建议。
 */
export interface SuggestedModelPrice {
  input: string
  output: string
  cachedInput: string
  cachedOutput: string
}

interface SuggestedModelEntry {
  /** 归一化（小写）后的精确匹配名。 */
  matches: string[]
  /** 归一化后以该前缀开头的模型名（用于带日期或版本后缀的型号）。 */
  prefixes?: string[]
  prices: SuggestedModelPrice
}

function entry(matches: string[], prices: [string, string, string?, string?], prefixes?: string[]): SuggestedModelEntry {
  return {
    matches,
    prefixes,
    prices: {
      input: prices[0],
      output: prices[1],
      cachedInput: prices[2] ?? '0',
      cachedOutput: prices[3] ?? '0',
    },
  }
}

const suggestedModelPrices: SuggestedModelEntry[] = [
  // OpenAI（https://openai.com/api/pricing/）
  entry(['gpt-5.2'], ['175', '1400', '17.5'], ['gpt-5.2-']),
  entry(['gpt-5.1'], ['125', '1000', '12.5'], ['gpt-5.1-']),
  entry(['gpt-5', 'gpt-5-2025-08-07'], ['125', '1000', '12.5']),
  entry(['gpt-5-mini'], ['25', '200', '2.5'], ['gpt-5-mini-']),
  entry(['gpt-5-nano'], ['5', '40', '0.5'], ['gpt-5-nano-']),
  entry(['gpt-5-pro', 'gpt-5-pro-2025-10-06'], ['1500', '12000']),
  entry(['gpt-4.1'], ['200', '800', '50'], ['gpt-4.1-']),
  entry(['gpt-4.1-mini'], ['40', '160', '10'], ['gpt-4.1-mini-']),
  entry(['gpt-4.1-nano'], ['10', '40', '2.5'], ['gpt-4.1-nano-']),
  entry(['gpt-4o', 'gpt-4o-2024-08-06', 'gpt-4o-2024-11-20', 'gpt-4o-2024-05-13'], ['250', '1000', '125']),
  entry(['gpt-4o-mini'], ['15', '60', '7.5'], ['gpt-4o-mini-']),
  entry(['o3', 'o3-2025-04-16'], ['200', '800', '50']),
  entry(['o4-mini', 'o4-mini-2025-04-16'], ['110', '440', '27.5']),

  // Anthropic Claude（https://claude.com/pricing；缓存读取为输入价 10%，缓存写入不单独计费）
  entry(['claude-opus-4.6', 'claude-opus-4-6'], ['500', '2500', '50'], ['claude-opus-4-6-']),
  entry(['claude-opus-4.5', 'claude-opus-4-5'], ['500', '2500', '50'], ['claude-opus-4-5-']),
  entry(['claude-opus-4.1', 'claude-opus-4-1'], ['1500', '7500', '150'], ['claude-opus-4-1-']),
  entry(['claude-opus-4', 'claude-opus-4-0'], ['1500', '7500', '150'], ['claude-opus-4-']),
  entry(['claude-sonnet-4.6', 'claude-sonnet-4-6'], ['300', '1500', '30'], ['claude-sonnet-4-6-']),
  entry(['claude-sonnet-4.5', 'claude-sonnet-4-5'], ['300', '1500', '30'], ['claude-sonnet-4-5-']),
  entry(['claude-sonnet-4', 'claude-sonnet-4-0'], ['300', '1500', '30'], ['claude-sonnet-4-20250514']),
  entry(['claude-haiku-4.5', 'claude-haiku-4-5'], ['100', '500', '10'], ['claude-haiku-4-5-']),
  entry(['claude-3.5-sonnet', 'claude-3-5-sonnet-20241022', 'claude-3-5-sonnet-20240620'], ['300', '1500', '30']),
  entry(['claude-3.5-haiku', 'claude-3-5-haiku-20241022'], ['80', '400', '8']),

  // Google Gemini（https://ai.google.dev/gemini-api/docs/pricing）
  entry(['gemini-3-pro', 'gemini-3-pro-preview'], ['200', '1200', '20']),
  entry(['gemini-3-flash', 'gemini-3-flash-preview'], ['50', '300', '5']),
  entry(['gemini-2.5-pro'], ['125', '1000', '12.5']),
  entry(['gemini-2.5-flash'], ['30', '250', '3']),
  entry(['gemini-2.5-flash-lite'], ['10', '40', '1']),

  // DeepSeek（https://api-docs.deepseek.com/quick_start/pricing；缓存价即官方缓存命中价）
  entry(['deepseek-chat'], ['28', '42', '2.8']),
  entry(['deepseek-reasoner'], ['55', '219', '5.5']),

  // 阿里云百炼（https://help.aliyun.com/zh/model-studio/models）
  entry(['qwen3-max', 'qwen3-max-preview'], ['120', '600']),
  entry(['qwen-plus', 'qwen-plus-latest'], ['80', '200']),
  entry(['qwen-flash'], ['15', '150']),
  entry(['qwen3-235b-a22b'], ['70', '280']),
  entry(['qwen3-32b'], ['40', '80']),

  // Moonshot Kimi（https://platform.moonshot.ai/docs/pricing/chat；缓存价即官方缓存命中价）
  entry(['kimi-k2.5', 'kimi-k2-5'], ['400', '2000', '70'], ['kimi-k2.5-']),
  entry(['kimi-k2-0905-preview', 'kimi-k2'], ['400', '1600', '100']),
  entry(['kimi-k2-turbo-preview'], ['800', '3200', '200']),
  entry(['kimi-k2-thinking', 'kimi-k2-thinking-turbo'], ['600', '2500', '100']),

  // 字节跳动豆包 / 火山方舟（https://www.volcengine.com/docs/82379/1544106）
  entry(['doubao-seed-1.6', 'doubao-seed-1-6-250615'], ['80', '800']),
  entry(['doubao-seed-1.6-flash', 'doubao-seed-1-6-flash-250715'], ['15', '150']),
  entry(['doubao-seed-1.6-lite', 'doubao-seed-1-6-lite-251015'], ['30', '300']),

  // 智谱 GLM（https://docs.bigmodel.cn/cn/guide/models）
  entry(['glm-5'], ['600', '2200']),
  entry(['glm-4.7'], ['400', '1800']),
  entry(['glm-4.6'], ['200', '600']),
  entry(['glm-4.5'], ['200', '600'], ['glm-4.5-']),
  entry(['glm-4.5-flash'], ['0', '0']),
]

/**
 * 按 Provider 模型名查找建议价格。先精确匹配，再按前缀匹配带日期/版本
 * 后缀的型号；未收录时返回 `undefined`。
 */
export function findSuggestedModelPrice(name: string): SuggestedModelPrice | undefined {
  const normalized = name.trim().toLowerCase()
  if (!normalized) {
    return undefined
  }
  for (const item of suggestedModelPrices) {
    if (item.matches.includes(normalized)) {
      return item.prices
    }
  }
  for (const item of suggestedModelPrices) {
    if (item.prefixes?.some(prefix => normalized.startsWith(prefix))) {
      return item.prices
    }
  }
  return undefined
}
