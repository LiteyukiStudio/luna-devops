import type { AccessToken } from '../../api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Plus, ShieldX } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '../../api/client'
import { MotionItem, MotionList } from '../../components/common/motion'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input, Select } from '../../components/ui/input'
import { Badge } from '../../components/ui/status'

const schema = z.object({
  name: z.string().min(1),
  scope: z.string().min(1),
  expiresInDays: z.coerce.number().int().min(0),
})

type TokenFormInput = z.input<typeof schema>
type TokenForm = z.output<typeof schema>

export function AccessTokensPage() {
  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-2xl font-semibold">Access Token</h1>
        <p className="mt-1 text-sm text-muted-foreground">用于 API 触发构建或部署，不使用 JWT，后端只保存 hash。</p>
      </div>
      <AccessTokensPanel />
    </div>
  )
}

export function AccessTokensPanel() {
  const queryClient = useQueryClient()
  const [createdToken, setCreatedToken] = useState('')
  const tokens = useQuery({ queryKey: ['access-tokens'], queryFn: api.listAccessTokens })
  const form = useForm<TokenFormInput, undefined, TokenForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      scope: 'build:trigger',
      expiresInDays: 30,
    },
  })

  const createToken = useMutation({
    mutationFn: api.createAccessToken,
    onSuccess: (result) => {
      setCreatedToken(result.accessToken)
      toast.success('Token 已创建，只会展示一次')
      form.reset()
      queryClient.invalidateQueries({ queryKey: ['access-tokens'] })
    },
    onError: error => toast.error(error.message),
  })

  const revokeToken = useMutation({
    mutationFn: api.revokeAccessToken,
    onSuccess: () => {
      toast.success('Token 已撤销')
      queryClient.invalidateQueries({ queryKey: ['access-tokens'] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-4 lg:grid-cols-[360px_1fr]">
      <Card>
        <h2 className="mb-4 text-base font-semibold">创建 Token</h2>
        <form className="grid gap-3" onSubmit={form.handleSubmit(values => createToken.mutate(values))}>
          <Field error={form.formState.errors.name?.message} label="名称" required>
            <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder="Git webhook trigger" />
          </Field>
          <Field error={form.formState.errors.scope?.message} label="Scope" required>
            <Select {...form.register('scope')} aria-invalid={Boolean(form.formState.errors.scope)}>
              <option value="build:trigger">build:trigger</option>
              <option value="deploy:trigger">deploy:trigger</option>
              <option value="project:read">project:read</option>
            </Select>
          </Field>
          <Field error={form.formState.errors.expiresInDays?.message} label="有效期天数" required>
            <Input type="number" {...form.register('expiresInDays')} aria-invalid={Boolean(form.formState.errors.expiresInDays)} />
          </Field>
          <Button disabled={createToken.isPending || !form.formState.isValid} type="submit">
            <Plus size={16} />
            创建 Token
          </Button>
        </form>
        {createdToken && (
          <div className="mt-4 rounded-md border border-border bg-muted p-3">
            <p className="mb-2 text-xs font-medium text-muted-foreground">仅展示一次</p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate text-xs">{createdToken}</code>
              <Button variant="secondary" onClick={() => navigator.clipboard.writeText(createdToken)}>
                <Copy size={14} />
              </Button>
            </div>
          </div>
        )}
      </Card>

      <MotionList className="grid gap-3">
        {(tokens.data ?? []).map(token => (
          <MotionItem key={token.id}>
            <TokenRow onRevoke={() => revokeToken.mutate(token.id)} token={token} />
          </MotionItem>
        ))}
        {tokens.data?.length === 0 && <Card className="text-sm text-muted-foreground">还没有 Token。</Card>}
      </MotionList>
    </div>
  )
}

function TokenRow({ token, onRevoke }: { token: AccessToken, onRevoke: () => void }) {
  return (
    <Card className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <KeyRound size={18} />
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate font-medium">{token.name}</h3>
            <Badge>{token.scope}</Badge>
            {token.revokedAt && <Badge className="text-danger">revoked</Badge>}
          </div>
          <p className="truncate text-sm text-muted-foreground">
            创建于
            {new Date(token.createdAt).toLocaleString()}
          </p>
        </div>
      </div>
      <Button disabled={Boolean(token.revokedAt)} variant="ghost" onClick={onRevoke}>
        <ShieldX size={16} />
        撤销
      </Button>
    </Card>
  )
}
