import type { TFunction } from 'i18next'
import type { AIUIAction } from '@/api'

interface PresetSuggestion {
  id: string
  labelKey: string
  messageKey: string
  tone?: 'default' | 'primary'
}

interface PresetGroup {
  matches: (pathname: string) => boolean
  suggestions: PresetSuggestion[]
}

export interface AISuggestionSet {
  actions: AIUIAction[]
  sourceKey: string
}

const presetGroups: PresetGroup[] = [
  preset('/dashboard', 'dashboard', ['overview', 'failures', 'projects', 'events']),
  preset(/^\/projects\/[^/]+\/apps\/[^/]+$/, 'application', ['status', 'releases', 'logs', 'config']),
  preset(/^\/projects\/[^/]+$/, 'project', ['applications', 'deploy', 'health', 'gateway']),
  preset('/projects', 'projects', ['list', 'create', 'deploy']),
  preset('/events', 'events', ['errors', 'recent', 'diagnose']),
  preset('/code-repositories', 'repositories', ['connected', 'add', 'deploy']),
  preset('/registries', 'registries', ['status', 'add', 'images']),
  preset('/clusters', 'clusters', ['health', 'resources', 'diagnose']),
  preset('/app-templates', 'templates', ['search', 'databases', 'deploy']),
  preset('/billing', 'billing', ['usage', 'costs', 'alerts']),
  preset('/settings/account', 'account', ['profile', 'security', 'tokens']),
  preset('/settings/auth-providers', 'authProviders', ['status', 'configure', 'diagnose']),
  preset('/settings/notifications', 'notifications', ['channels', 'rules', 'test']),
  preset('/settings/operations', 'operations', ['health', 'retention', 'events']),
  preset('/settings/site', 'site', ['assistant', 'policies', 'configuration']),
  preset('/settings/users', 'users', ['list', 'permissions', 'invite']),
]

const fallbackSuggestions = suggestions('general', ['overview', 'diagnose', 'guide'])

export function resolveAIPresetSuggestions(pathname: string, t: TFunction, allowPresets: boolean): AISuggestionSet | null {
  if (!allowPresets)
    return null

  const group = presetGroups.find(candidate => candidate.matches(pathname))
  const presets = group?.suggestions ?? fallbackSuggestions
  return {
    actions: presets.map(item => ({
      version: 1,
      id: `preset-${item.id}`,
      repeatable: false,
      type: 'send_message',
      label: t(`${item.labelKey}.label`),
      tone: item.tone ?? 'default',
      payload: { message: t(`${item.messageKey}.message`) },
    })),
    sourceKey: `preset:${group ? pathname : 'general'}`,
  }
}

function preset(path: string | RegExp, group: string, items: string[]): PresetGroup {
  return {
    matches: typeof path === 'string' ? pathname => pathname === path : pathname => path.test(pathname),
    suggestions: suggestions(group, items),
  }
}

function suggestions(group: string, items: string[]): PresetSuggestion[] {
  return items.map((item, index) => ({
    id: `${group}-${item}`,
    labelKey: `aiAssistant.options.presets.${group}.${item}`,
    messageKey: `aiAssistant.options.presets.${group}.${item}`,
    tone: index === 0 ? 'primary' : 'default',
  }))
}
