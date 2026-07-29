import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { AIMarkdown } from './markdown'

function renderMarkdown(markdown: string) {
  return render(
    <MemoryRouter initialEntries={['/dashboard']}>
      <AIMarkdown>{markdown}</AIMarkdown>
      <LocationProbe />
    </MemoryRouter>,
  )
}

function LocationProbe() {
  const location = useLocation()
  return <output data-testid="location">{`${location.pathname}${location.search}${location.hash}`}</output>
}

describe('ai markdown', () => {
  it('renders GFM tables and compact rich text semantics', () => {
    const { container } = renderMarkdown(`## 状态摘要

- 正常项目：2
- 异常项目：0

| 项目空间 | 状态 |
| --- | --- |
| Luna Platform | 正常 |

使用 \`luna-devops\`。`)

    expect(screen.getByRole('heading', { name: '状态摘要' })).toBeInTheDocument()
    expect(screen.getByRole('list')).toBeInTheDocument()
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'Luna Platform' })).toBeInTheDocument()
    expect(screen.getByText('luna-devops')).toHaveProperty('tagName', 'CODE')
    expect(container.querySelector('[data-slot="ai-markdown-table-scroll"]')).toHaveClass('overflow-x-auto', 'overscroll-x-contain')
  })

  it('renders fenced code and safe external links without raw HTML', () => {
    const { container } = renderMarkdown('[文档](https://example.com)\n\n```bash\npnpm --dir web build\n```\n\n<script>alert("unsafe")</script>')

    expect(screen.getByRole('link', { name: '文档' })).toHaveAttribute('target', '_blank')
    expect(screen.getByRole('link', { name: '文档' })).toHaveAttribute('rel', 'noreferrer noopener')
    expect(screen.getByText('pnpm --dir web build')).toHaveProperty('tagName', 'CODE')
    expect(container.querySelector('[data-slot="ai-markdown-code-scroll"]')).toHaveClass('overflow-x-auto', 'overscroll-x-contain')
    expect(container.querySelector('script')).not.toBeInTheDocument()
    expect(screen.queryByText('alert("unsafe")')).not.toBeInTheDocument()
  })

  it('renders registered internal routes in the primary color and navigates without reloading', () => {
    renderMarkdown('[查看构建](/projects/prj_1/apps/app_1?tab=builds#tab=builds&buildRunId=bldr_1)')

    const link = screen.getByRole('link', { name: '查看构建' })
    expect(link).toHaveClass('text-primary-text')
    expect(link).toHaveAttribute('data-slot', 'ai-markdown-internal-link')
    expect(link).not.toHaveAttribute('target')
    fireEvent.click(link)
    expect(screen.getByTestId('location')).toHaveTextContent('/projects/prj_1/apps/app_1?tab=builds#tab=builds&buildRunId=bldr_1')
  })

  it('does not create clickable links for unsafe or unregistered destinations', () => {
    renderMarkdown('[脚本](javascript:alert(1)) [未知页面](/not-registered) [协议相对](//evil.example)')

    expect(screen.queryByRole('link', { name: '脚本' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '未知页面' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '协议相对' })).not.toBeInTheDocument()
  })
})
