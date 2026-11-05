import type { ReactNode } from 'react'
import type { Components } from 'react-markdown'
import { isValidElement, memo } from 'react'
import Markdown from 'react-markdown'
import { Link } from 'react-router-dom'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'
import { CopyableCodeBlock } from './copyable-code-block'
import { normalizeAIExternalHref, normalizeAIInternalHref } from './internal-routes'

const linkClassName = 'font-medium text-primary-text underline decoration-primary/40 underline-offset-2 transition-colors hover:text-primary-text-strong hover:decoration-primary focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

function codeLanguage(children: ReactNode): string | undefined {
  const child = Array.isArray(children) ? children[0] : children
  if (!isValidElement<{ className?: string }>(child))
    return undefined
  return child.props.className?.match(/language-([\w+#.-]+)/)?.[1]
}

const markdownComponents: Components = {
  a: ({ children, href, title }) => {
    const internalHref = normalizeAIInternalHref(href)
    if (internalHref) {
      return (
        <Link className={linkClassName} data-slot="ai-markdown-internal-link" title={title} to={internalHref}>
          {children}
        </Link>
      )
    }
    const externalHref = normalizeAIExternalHref(href)
    if (!externalHref)
      return <span>{children}</span>
    return (
      <a className={linkClassName} href={externalHref} rel="noreferrer noopener" target="_blank" title={title}>
        {children}
      </a>
    )
  },
  blockquote: ({ children }) => (
    <blockquote className="my-3 border-l-2 border-border pl-3 text-muted-foreground">
      {children}
    </blockquote>
  ),
  code: ({ children, className }) => (
    <code className={cn('break-words rounded-[0.3rem] border border-border bg-muted px-1 py-px font-mono text-[0.86em]', className)}>
      {children}
    </code>
  ),
  h1: ({ children }) => <h1 className="mb-2 mt-5 text-[1.15em] font-semibold leading-snug first:mt-0">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-2 mt-4 text-[1.08em] font-semibold leading-snug first:mt-0">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1.5 mt-3.5 font-semibold first:mt-0">{children}</h3>,
  h4: ({ children }) => <h4 className="mb-1 mt-3 font-medium first:mt-0">{children}</h4>,
  hr: () => <hr className="my-4 border-border" />,
  img: ({ alt }) => alt ? <span className="text-muted-foreground">{alt}</span> : null,
  li: ({ children }) => <li className="my-1 pl-1 marker:text-muted-foreground">{children}</li>,
  ol: ({ children }) => <ol className="my-2.5 list-decimal pl-5">{children}</ol>,
  p: ({ children }) => <p className="my-2.5 first:mt-0 last:mb-0">{children}</p>,
  pre: ({ children }) => (
    <div className="my-3 w-full min-w-0 first:mt-0 last:mb-0">
      <CopyableCodeBlock className="overscroll-x-contain [&_code]:break-normal [&_code]:border-0 [&_code]:bg-transparent [&_code]:p-0 [&_code]:text-inherit" dataSlot="ai-markdown-code-scroll" language={codeLanguage(children)}>
        {children}
      </CopyableCodeBlock>
    </div>
  ),
  table: ({ children }) => (
    <div className="my-3 w-full min-w-0 max-w-full overflow-x-auto overscroll-x-contain rounded-control border border-border first:mt-0 last:mb-0" data-slot="ai-markdown-table-scroll">
      <table className="w-max min-w-full border-collapse text-left text-xs">{children}</table>
    </div>
  ),
  tbody: ({ children }) => <tbody className="[&_tr:last-child]:border-b-0">{children}</tbody>,
  td: ({ children }) => <td className="max-w-64 px-3 py-2 align-top [overflow-wrap:anywhere]">{children}</td>,
  th: ({ children }) => <th className="whitespace-nowrap bg-muted px-3 py-2 font-medium text-muted-foreground">{children}</th>,
  tr: ({ children }) => <tr className="border-b border-border">{children}</tr>,
  ul: ({ children }) => <ul className="my-2.5 list-disc pl-5">{children}</ul>,
}

function AIMarkdownContent({ children, className }: { children: string, className?: string }) {
  return (
    <div className={cn('min-w-0 [overflow-wrap:anywhere] text-[13px] leading-5.5', className)}>
      <Markdown components={markdownComponents} remarkPlugins={[remarkGfm]} skipHtml>
        {children}
      </Markdown>
    </div>
  )
}

export const AIMarkdown = memo(AIMarkdownContent)

function AIInlineMarkdownContent({ children, className }: { children: string, className?: string }) {
  return (
    <span className={cn('min-w-0 [overflow-wrap:anywhere]', className)}>
      <Markdown
        allowedElements={['a', 'br', 'code', 'del', 'em', 'strong']}
        components={markdownComponents}
        remarkPlugins={[remarkGfm]}
        skipHtml
        unwrapDisallowed
      >
        {children}
      </Markdown>
    </span>
  )
}

export const AIInlineMarkdown = memo(AIInlineMarkdownContent)
