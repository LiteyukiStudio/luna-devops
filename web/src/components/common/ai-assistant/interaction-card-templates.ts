import type { InteractionCardGroup } from './interaction-card-schema'

export type InteractionCardTemplate = InteractionCardGroup['template']
export type InteractionCardDensity = NonNullable<NonNullable<InteractionCardGroup['display']>['density']>

interface InteractionCardTemplateConfig {
  defaultDensity: InteractionCardDensity
  expandByDefault: boolean
  gridClassName: string
}

export const interactionCardTemplateConfigs: Record<InteractionCardTemplate, InteractionCardTemplateConfig> = {
  catalog: {
    defaultDensity: 'comfortable',
    expandByDefault: false,
    gridClassName: 'grid-cols-[repeat(auto-fit,minmax(min(17rem,100%),1fr))]',
  },
  comparison: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-[repeat(auto-fit,minmax(min(16rem,100%),1fr))]',
  },
  inspector: {
    defaultDensity: 'comfortable',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  form: {
    defaultDensity: 'comfortable',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  wizard: {
    defaultDensity: 'comfortable',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  diagnosis: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  plan: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  progress: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  result: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  dashboard: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-[repeat(auto-fit,minmax(min(13rem,100%),1fr))]',
  },
}

export function interactionCardDensity(group: InteractionCardGroup): InteractionCardDensity {
  return group.display?.density ?? interactionCardTemplateConfigs[group.template].defaultDensity
}

export function shouldExpandInteractionCard(group: InteractionCardGroup, hasForm: boolean, explicitlyExpanded: boolean) {
  return hasForm || explicitlyExpanded || interactionCardTemplateConfigs[group.template].expandByDefault
}
