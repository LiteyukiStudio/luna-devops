// 兼容层：executor 已拆分为 executor/ 目录下的多个模块。
// 此文件保持原有导入路径不变，逐步迁移引用后可移除。
export { RunExecutor } from "./executor/index.js"
export { serializeToolResultPayload, setToolResultPayloadBudget } from "./executor/tool-results.js"
export { setMaxCardRepairAttempts } from "./executor/cards.js"
