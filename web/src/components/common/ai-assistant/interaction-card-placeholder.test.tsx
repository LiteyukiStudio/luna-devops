import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
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
})
