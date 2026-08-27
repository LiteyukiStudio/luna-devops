export const maximumTurnInputBytes = 128 * 1024

// Luna API 先限制 1 MiB 原始信封，再注入 Actor 绑定的 Run、页面上下文和模型快照。
// 内部上限只为这些有界服务端字段留出余量，用户文本仍受独立 128 KiB UTF-8 上限约束。
export const maximumRequestBodyBytes = 2 * 1024 * 1024

export function utf8ByteLength(value: string): number {
  return Buffer.byteLength(value, "utf8")
}
