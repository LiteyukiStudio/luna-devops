import type { DeploymentTargetFormValues } from '../deployment-target-form'
import { useId, useState } from 'react'
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { CodeEditor } from '@/components/common/code-editor'
import { FormField } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { useFieldError } from '../deployment-target-editor-utils'

type ProbeFieldName = 'readinessProbe' | 'livenessProbe' | 'startupProbe'
type ProbeMode = 'none' | 'httpGet' | 'tcpSocket' | 'exec' | 'json'

interface StructuredProbeDraft {
  commandText: string
  failureThreshold: number
  initialDelaySeconds: number
  mode: Exclude<ProbeMode, 'none' | 'json'>
  path: string
  periodSeconds: number
  port: number
  scheme: 'HTTP' | 'HTTPS'
  timeoutSeconds: number
}

const allowedTopKeys = new Set([
  'httpGet',
  'tcpSocket',
  'exec',
  'initialDelaySeconds',
  'periodSeconds',
  'timeoutSeconds',
  'failureThreshold',
  'successThreshold',
])

const allowedHttpKeys = new Set(['path', 'port', 'scheme'])
const allowedTcpKeys = new Set(['port'])
const allowedExecKeys = new Set(['command'])

function parseStructuredProbe(raw: string, fallbackPort: number): { mode: ProbeMode, draft: StructuredProbeDraft } {
  const baseDraft: StructuredProbeDraft = {
    commandText: '/bin/sh\n-c\ntest -f /tmp/healthy',
    failureThreshold: 3,
    initialDelaySeconds: 5,
    mode: 'httpGet',
    path: '/healthz',
    periodSeconds: 10,
    port: fallbackPort,
    scheme: 'HTTP',
    timeoutSeconds: 3,
  }
  const trimmed = raw.trim()
  if (!trimmed)
    return { mode: 'none', draft: baseDraft }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  }
  catch {
    return { mode: 'json', draft: baseDraft }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
    return { mode: 'json', draft: baseDraft }

  const record = parsed as Record<string, unknown>
  if (Object.keys(record).some(key => !allowedTopKeys.has(key)))
    return { mode: 'json', draft: baseDraft }

  const actionCount = ['httpGet', 'tcpSocket', 'exec'].filter(key => record[key] !== undefined).length
  if (actionCount !== 1)
    return { mode: 'json', draft: baseDraft }

  const timing = {
    failureThreshold: positiveInt(record.failureThreshold, 3),
    initialDelaySeconds: nonNegativeInt(record.initialDelaySeconds, 0),
    periodSeconds: positiveInt(record.periodSeconds, 10),
    timeoutSeconds: positiveInt(record.timeoutSeconds, 1),
  }

  if (record.httpGet && typeof record.httpGet === 'object' && !Array.isArray(record.httpGet)) {
    const http = record.httpGet as Record<string, unknown>
    if (Object.keys(http).some(key => !allowedHttpKeys.has(key)))
      return { mode: 'json', draft: baseDraft }
    const port = typeof http.port === 'number' ? http.port : Number(http.port)
    if (!Number.isInteger(port) || port <= 0)
      return { mode: 'json', draft: baseDraft }
    const scheme = http.scheme === 'HTTPS' ? 'HTTPS' : 'HTTP'
    return {
      mode: 'httpGet',
      draft: {
        ...baseDraft,
        ...timing,
        mode: 'httpGet',
        path: typeof http.path === 'string' ? http.path : '/',
        port,
        scheme,
      },
    }
  }

  if (record.tcpSocket && typeof record.tcpSocket === 'object' && !Array.isArray(record.tcpSocket)) {
    const tcp = record.tcpSocket as Record<string, unknown>
    if (Object.keys(tcp).some(key => !allowedTcpKeys.has(key)))
      return { mode: 'json', draft: baseDraft }
    const port = typeof tcp.port === 'number' ? tcp.port : Number(tcp.port)
    if (!Number.isInteger(port) || port <= 0)
      return { mode: 'json', draft: baseDraft }
    return {
      mode: 'tcpSocket',
      draft: {
        ...baseDraft,
        ...timing,
        mode: 'tcpSocket',
        port,
      },
    }
  }

  if (record.exec && typeof record.exec === 'object' && !Array.isArray(record.exec)) {
    const execObj = record.exec as Record<string, unknown>
    if (Object.keys(execObj).some(key => !allowedExecKeys.has(key)))
      return { mode: 'json', draft: baseDraft }
    if (!Array.isArray(execObj.command) || execObj.command.some(item => typeof item !== 'string'))
      return { mode: 'json', draft: baseDraft }
    return {
      mode: 'exec',
      draft: {
        ...baseDraft,
        ...timing,
        commandText: (execObj.command as string[]).join('\n'),
        mode: 'exec',
      },
    }
  }

  return { mode: 'json', draft: baseDraft }
}

function serializeStructuredProbe(draft: StructuredProbeDraft): string {
  const payload: Record<string, unknown> = {}
  if (draft.mode === 'httpGet') {
    payload.httpGet = {
      path: draft.path,
      port: Math.max(1, Math.min(65535, Math.trunc(draft.port) || 8080)),
      ...(draft.scheme === 'HTTPS' ? { scheme: 'HTTPS' } : {}),
    }
  }
  else if (draft.mode === 'tcpSocket') {
    payload.tcpSocket = {
      port: Math.max(1, Math.min(65535, Math.trunc(draft.port) || 8080)),
    }
  }
  else {
    const command = draft.commandText
      .split('\n')
      .map(line => line.trim())
      .filter(Boolean)
    payload.exec = {
      command: command.length > 0 ? command : ['/bin/true'],
    }
  }
  if (draft.initialDelaySeconds > 0)
    payload.initialDelaySeconds = Math.trunc(draft.initialDelaySeconds)
  payload.periodSeconds = Math.max(1, Math.trunc(draft.periodSeconds) || 10)
  payload.timeoutSeconds = Math.max(1, Math.trunc(draft.timeoutSeconds) || 3)
  payload.failureThreshold = Math.max(1, Math.trunc(draft.failureThreshold) || 3)
  return JSON.stringify(payload)
}

function nonNegativeInt(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0 ? value : fallback
}

function positiveInt(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : fallback
}

export function ProbeFieldEditor({
  defaultPort = 8080,
  hint,
  label,
  name,
  placeholder,
}: {
  defaultPort?: number
  hint: string
  label: string
  name: ProbeFieldName
  placeholder: string
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const error = useFieldError(name)
  const modeSelectId = useId()
  const [forceJsonMode, setForceJsonMode] = useState(false)

  return (
    <Controller
      control={control}
      name={name}
      render={({ field }) => {
        const parsed = parseStructuredProbe(field.value, defaultPort)
        const effectiveMode: ProbeMode = forceJsonMode && field.value.trim() !== '' ? 'json' : parsed.mode
        const draft = parsed.draft

        const handleModeChange = (nextMode: ProbeMode) => {
          if (nextMode === 'none') {
            setForceJsonMode(false)
            field.onChange('')
            return
          }
          if (nextMode === 'json') {
            setForceJsonMode(true)
            if (!field.value.trim())
              field.onChange(placeholder)
            return
          }
          setForceJsonMode(false)
          const nextDraft: StructuredProbeDraft = {
            ...draft,
            mode: nextMode,
            port: draft.port || defaultPort,
          }
          field.onChange(serializeStructuredProbe(nextDraft))
        }

        const updateDraft = (patch: Partial<StructuredProbeDraft>) => {
          const nextDraft = { ...draft, ...patch }
          field.onChange(serializeStructuredProbe(nextDraft))
        }

        return (
          <div className="grid gap-3 rounded-control border border-border bg-surface-subtle/30 p-3.5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1.5">
                <label className="text-sm font-medium text-foreground" htmlFor={modeSelectId}>{label}</label>
                <HelpTip label={label}>{hint}</HelpTip>
              </span>
              <NativeSelect
                aria-label={label}
                className="h-8 w-44 text-xs"
                containerClassName="w-auto"
                id={modeSelectId}
                value={effectiveMode}
                onChange={event => handleModeChange(event.target.value as ProbeMode)}
              >
                <option value="none">{t('deploymentsPage.probeTypes.none')}</option>
                <option value="httpGet">{t('deploymentsPage.probeTypes.httpGet')}</option>
                <option value="tcpSocket">{t('deploymentsPage.probeTypes.tcpSocket')}</option>
                <option value="exec">{t('deploymentsPage.probeTypes.exec')}</option>
                <option value="json">{t('deploymentsPage.probeTypes.json')}</option>
              </NativeSelect>
            </div>

            {effectiveMode === 'httpGet' && (
              <div className="grid gap-3 @xl/workspace:grid-cols-3">
                <FormField label={t('deploymentsPage.probeHttpPath')}>
                  <Input
                    placeholder="/healthz"
                    value={draft.path}
                    onBlur={() => {
                      if (!draft.path.trim())
                        updateDraft({ path: '/' })
                    }}
                    onChange={event => updateDraft({ path: event.target.value })}
                  />
                </FormField>
                <FormField label={t('deploymentsPage.servicePortNumber')}>
                  <Input
                    max={65535}
                    min={1}
                    type="number"
                    value={draft.port}
                    onChange={event => updateDraft({ port: Number(event.target.value) || defaultPort })}
                  />
                </FormField>
                <FormField label={t('deploymentsPage.probeHttpScheme')}>
                  <NativeSelect
                    value={draft.scheme}
                    onChange={event => updateDraft({ scheme: event.target.value === 'HTTPS' ? 'HTTPS' : 'HTTP' })}
                  >
                    <option value="HTTP">HTTP</option>
                    <option value="HTTPS">HTTPS</option>
                  </NativeSelect>
                </FormField>
              </div>
            )}

            {effectiveMode === 'tcpSocket' && (
              <div className="grid gap-3 @xl/workspace:grid-cols-3">
                <FormField label={t('deploymentsPage.servicePortNumber')}>
                  <Input
                    max={65535}
                    min={1}
                    type="number"
                    value={draft.port}
                    onChange={event => updateDraft({ port: Number(event.target.value) || defaultPort })}
                  />
                </FormField>
              </div>
            )}

            {effectiveMode === 'exec' && (
              <FormField hint={t('deploymentsPage.probeExecCommandHint')} label={t('deploymentsPage.probeExecCommand')}>
                <Textarea
                  className="min-h-20 font-mono text-sm"
                  spellCheck={false}
                  value={draft.commandText}
                  onChange={event => updateDraft({ commandText: event.target.value })}
                />
              </FormField>
            )}

            {(effectiveMode === 'httpGet' || effectiveMode === 'tcpSocket' || effectiveMode === 'exec') && (
              <div className="grid gap-3 @xl/workspace:grid-cols-4">
                <FormField label={t('deploymentsPage.probeInitialDelaySeconds')}>
                  <Input
                    min={0}
                    type="number"
                    value={draft.initialDelaySeconds}
                    onChange={event => updateDraft({ initialDelaySeconds: Math.max(0, Number(event.target.value) || 0) })}
                  />
                </FormField>
                <FormField label={t('deploymentsPage.probePeriodSeconds')}>
                  <Input
                    min={1}
                    type="number"
                    value={draft.periodSeconds}
                    onChange={event => updateDraft({ periodSeconds: Math.max(1, Number(event.target.value) || 10) })}
                  />
                </FormField>
                <FormField label={t('deploymentsPage.probeTimeoutSeconds')}>
                  <Input
                    min={1}
                    type="number"
                    value={draft.timeoutSeconds}
                    onChange={event => updateDraft({ timeoutSeconds: Math.max(1, Number(event.target.value) || 3) })}
                  />
                </FormField>
                <FormField label={t('deploymentsPage.probeFailureThreshold')}>
                  <Input
                    min={1}
                    type="number"
                    value={draft.failureThreshold}
                    onChange={event => updateDraft({ failureThreshold: Math.max(1, Number(event.target.value) || 3) })}
                  />
                </FormField>
              </div>
            )}

            {effectiveMode === 'json' && (
              <FormField error={error} label={t('deploymentsPage.probeTypes.json')}>
                {(fieldControl, { labelId }) => (
                  <CodeEditor
                    ariaDescribedBy={fieldControl['aria-describedby']}
                    ariaInvalid={Boolean(error)}
                    ariaLabelledBy={labelId}
                    language="json"
                    minHeight="7rem"
                    placeholder={t('deploymentTargetEditor.examplePlaceholder', { value: placeholder })}
                    value={field.value}
                    onChange={field.onChange}
                  />
                )}
              </FormField>
            )}

            {effectiveMode !== 'json' && error && (
              <p className="text-xs text-destructive" role="alert">{error}</p>
            )}
          </div>
        )
      }}
    />
  )
}
