import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { KeyValueTextEditor } from './key-value-text-editor'
import '@/i18n'

describe('key value text editor', () => {
  it('localizes a line without the KEY=VALUE separator by stable error code', () => {
    render(<KeyValueTextEditor initialValue={{}} onChange={vi.fn()} />)

    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'MISSING_SEPARATOR' } })

    expect(screen.getByRole('alert')).toHaveTextContent(/请按每行 KEY=VALUE 的格式填写。|Use one KEY=VALUE entry per line./)
    expect(screen.queryByText(/common\.key_value_line_missing_separator/)).not.toBeInTheDocument()
  })
})
