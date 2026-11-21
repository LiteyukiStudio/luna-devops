/**
 * Radix Select 禁止 `<SelectItem value="">` 空字符串值；
 * 当业务需要表达“全部 / 无 / 默认”等合法空字符串选项时，使用此哨兵常量与转换辅助函数。
 */
export const EMPTY_SELECT_VALUE = '__empty__'

export function toSelectValue(value: string | null | undefined): string {
  return value || EMPTY_SELECT_VALUE
}

export function fromSelectValue(value: string): string {
  return value === EMPTY_SELECT_VALUE ? '' : value
}
