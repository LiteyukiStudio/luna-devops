import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { CodeEditor } from './code-editor'

vi.mock('./code-editor-core', () => ({
  CodeEditorCore: () => {
    throw new Error('editor extension failed')
  },
}))

beforeAll(async () => {
  await i18next.changeLanguage('zh-CN')
})

describe('code editor fallback', () => {
  afterEach(() => vi.restoreAllMocks())

  it('keeps the basic text input editable when the enhanced editor fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const onChange = vi.fn()
    render(<CodeEditor language="yaml" value="apiVersion: v1" onChange={onChange} />)

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('高级编辑器暂时不可用'))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'kind: ConfigMap' } })

    expect(onChange).toHaveBeenCalledWith('kind: ConfigMap')
  })
})
