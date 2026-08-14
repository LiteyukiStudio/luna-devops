import { render, waitFor } from '@testing-library/react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { CodeEditor } from './code-editor'

beforeAll(async () => {
  await i18next.changeLanguage('zh-CN')
})

describe('code editor', () => {
  it.each([
    ['yaml', 'apiVersion: v1\nkind: Config'],
    ['json', '{"enabled":true}'],
    ['text', 'FROM scratch'],
  ] as const)('mounts the %s editor with its runtime extensions', async (language, value) => {
    const { container } = render(
      <CodeEditor language={language} value={value} onChange={vi.fn()} />,
    )

    await waitFor(() => expect(container.querySelector('.cm-editor')).toBeInTheDocument())
    expect(container.querySelector('.cm-content')).toHaveAttribute('contenteditable', 'true')
  })
})
