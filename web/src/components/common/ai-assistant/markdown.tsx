import type { Components } from 'react-markdown'
import { memo } from 'react'
import Markdown from 'react-markdown'
import { Link } from 'react-router-dom'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'
import { normalizeAIExternalHref, normalizeAIInternalHref } from './internal-routes'

const linkClassName = 'font-medium text-primary-text underline decoration-primary/40 underline-offset-2 transition-colors hover:text-primary-text-strong hover:decoration-primary focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

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
    <blockquote className="my-2 border-l-2 border-primary/40 pl-3 text-muted-foreground">
      {children}
    </blockquote>
  ),
  code: ({ children, className }) => (
    <code className={cn('break-words rounded-sm bg-surface-inset px-1 py-0.5 font-mono text-[0.84em]', className)}>
      {children}
    </code>
  ),
  h1: ({ children }) => <h1 className="mb-2 mt-3 text-base font-semibold leading-6 first:mt-0">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-1.5 mt-3 text-[15px] font-semibold leading-5.5 first:mt-0">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1 mt-2.5 text-sm font-semibold leading-5 first:mt-0">{children}</h3>,
  h4: ({ children }) => <h4 className="mb-1 mt-2 text-sm font-medium leading-5 first:mt-0">{children}</h4>,
  hr: () => <hr className="my-3 border-separator-subtle" />,
  img: ({ alt }) => alt ? <span className="text-muted-foreground">{alt}</span> : null,
  li: ({ children }) => <li className="my-0.5 pl-0.5 marker:text-muted-foreground">{children}</li>,
  ol: ({ children }) => <ol className="my-2 list-decimal space-y-0.5 pl-5">{children}</ol>,
  p: ({ children }) => <p className="my-2 first:mt-0 last:mb-0">{children}</p>,
  pre: ({ children }) => (
    <pre className="my-2 w-full min-w-0 max-w-full overflow-x-auto overscroll-x-contain rounded-control bg-surface-inset p-3 font-mono text-xs leading-5 text-foreground [&_code]:break-normal [&_code]:bg-transparent [&_code]:p-0 [&_code]:text-inherit" data-slot="ai-markdown-code-scroll">
      {children}
    </pre>
  ),
  table: ({ children }) => (
    <div className="my-2 w-full min-w-0 max-w-full overflow-x-auto overscroll-x-contain rounded-control border border-separator-subtle" data-slot="ai-markdown-table-scroll">
      <table className="w-max min-w-full border-collapse text-left text-xs">{children}</table>
    </div>
  ),
  tbody: ({ children }) => <tbody className="[&_tr:last-child]:border-b-0">{children}</tbody>,
  td: ({ children }) => <td className="max-w-56 border-r border-separator-subtle px-2.5 py-2 align-top [overflow-wrap:anywhere] last:border-r-0">{children}</td>,
  th: ({ children }) => <th className="whitespace-nowrap border-r border-separator-subtle bg-surface-inset px-2.5 py-2 font-medium last:border-r-0">{children}</th>,
  tr: ({ children }) => <tr className="border-b border-separator-subtle">{children}</tr>,
  ul: ({ children }) => <ul className="my-2 list-disc space-y-0.5 pl-5">{children}</ul>,
}

function AIMarkdownContent({ children, className }: { children: string, className?: string }) {
  return (
    <div className={cn('min-w-0 break-words text-sm leading-5.5', className)}>
      <Markdown components={markdownComponents} remarkPlugins={[remarkGfm]} skipHtml>
        {children}
      </Markdown>
    </div>
  )
}

export const AIMarkdown = memo(AIMarkdownContent)
