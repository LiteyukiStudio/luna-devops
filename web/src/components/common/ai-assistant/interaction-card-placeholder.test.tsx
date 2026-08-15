import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { AIInteractionCardPlaceholder } from './interaction-card-placeholder'

describe('aI interaction card placeholder', () => {
  it('renders the preparation title and safe markdown while the card is generated', () => {
    render(
      <AIInteractionCardPlaceholder
        arguments={{
          schemaVersion: 1,
          generationId: 'database-candidates',
          title: '正在整理 **数据库候选**',
          description: '正在组合版本、来源与配置字段。',
        }}
      />,
    )

    expect(screen.getByRole('status')).toHaveAttribute('data-ai-card-preparing')
    expect(screen.getByText('数据库候选').tagName).toBe('STRONG')
  })

  it('uses the localized fallback when the model has not supplied a title yet', async () => {
    await i18next.changeLanguage('en-US')
    render(
      <AIInteractionCardPlaceholder
        arguments={{
          schemaVersion: 1,
          generationId: 'database-candidates',
          placement: 'inline',
        }}
      />,
    )

    expect(screen.getByRole('status')).toHaveAccessibleName('Organizing card content')
  })
})
