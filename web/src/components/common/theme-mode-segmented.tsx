import type { ThemeMode } from '@/app/theme-context'
import { Monitor, Moon, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SegmentedControl } from './segmented-control'

const modes: Array<{ value: ThemeMode, labelKey: string, icon: typeof Sun }> = [
  { value: 'light', labelKey: 'theme.light', icon: Sun },
  { value: 'system', labelKey: 'theme.system', icon: Monitor },
  { value: 'dark', labelKey: 'theme.dark', icon: Moon },
]

/**
 * light/system/dark 三态主题切换器。
 * 用于设置页或侧边栏主题入口，文案走 i18n；不要用于业务状态或资源筛选。
 */
export function ThemeModeSegmented({ mode, setMode }: { mode: ThemeMode, setMode: (mode: ThemeMode) => void }) {
  const { t } = useTranslation()

  return (
    <SegmentedControl
      ariaLabel={t('theme.mode')}
      equalColumns
      items={modes.map(item => ({ value: item.value, label: t(item.labelKey), icon: item.icon }))}
      layoutId="theme-mode-active"
      showLabels={false}
      value={mode}
      onValueChange={setMode}
    />
  )
}
