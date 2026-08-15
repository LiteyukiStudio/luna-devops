import type { InteractionCardGroup } from './interaction-card-schema'

export type InteractionCardTemplate = InteractionCardGroup['template']
export type InteractionCardDensity = NonNullable<NonNullable<InteractionCardGroup['display']>['density']>

interface InteractionCardTemplateConfig {
  defaultDensity: InteractionCardDensity
  expandByDefault: boolean
  gridClassName: string
}

export const interactionCardTemplateConfigs: Record<InteractionCardTemplate, InteractionCardTemplateConfig> = {
  candidates: {
    defaultDensity: 'comfortable',
    expandByDefault: false,
    gridClassName: 'grid-cols-[repeat(auto-fit,minmax(min(17rem,100%),1fr))]',
  },
  form: {
    defaultDensity: 'comfortable',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  change_review: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  live_task: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
  result: {
    defaultDensity: 'compact',
    expandByDefault: true,
    gridClassName: 'grid-cols-1',
  },
}

export function interactionCardDensity(group: InteractionCardGroup): InteractionCardDensity {
  return group.display?.density ?? interactionCardTemplateConfigs[group.template].defaultDensity
}

export function shouldExpandInteractionCard(group: InteractionCardGroup, hasForm: boolean, explicitlyExpanded: boolean) {
  return hasForm || explicitlyExpanded || interactionCardTemplateConfigs[group.template].expandByDefault
}
