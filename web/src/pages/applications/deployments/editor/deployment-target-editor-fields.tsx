import type { ReactNode } from 'react'
import type { FieldPath } from 'react-hook-form'
import type { DeploymentTargetEditorSection, DeploymentTargetFormValues } from './deployment-target-form'
import { ChevronDown } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller, get, useFormContext, useFormState } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { CodeEditor } from '@/components/common/code-editor'
import { FormField } from '@/components/common/form-field'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { deploymentTargetSectionAnchor, useFieldError } from './deployment-target-editor-utils'

type BooleanFieldName = {
  [Key in keyof DeploymentTargetFormValues]: DeploymentTargetFormValues[Key] extends boolean ? Key : never
}[keyof DeploymentTargetFormValues]

type TextFieldName = {
  [Key in keyof DeploymentTargetFormValues]: DeploymentTargetFormValues[Key] extends string ? Key : never
}[keyof DeploymentTargetFormValues]

/** 编辑页的一级分区：带锚点，供左侧导航定位。 */
export function EditorSection({ children, description, section, title, tools }: {
  children: ReactNode
  description?: ReactNode
  section: DeploymentTargetEditorSection
  title: string
  tools?: ReactNode
}) {
  return (
    <section aria-labelledby={`${deploymentTargetSectionAnchor(section)}-title`} className="scroll-mt-24" id={deploymentTargetSectionAnchor(section)}>
      <Section
        className="gap-6"
        description={description}
        title={<span id={`${deploymentTargetSectionAnchor(section)}-title`}>{title}</span>}
        tools={tools}
        variant="bordered"
      >
        {children}
      </Section>
    </section>
  )
}

/** 分区内的二级分组，用细分隔线与标题区分同一分区下的不同任务。 */
export function EditorGroup({ children, className, description, title, tools }: {
  children: ReactNode
  className?: string
  description?: ReactNode
  title: string
  tools?: ReactNode
}) {
  return (
    <div className={cn('grid min-w-0 gap-4 border-t border-border pt-5 first:border-t-0 first:pt-0', className)}>
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        {tools && <div className="flex shrink-0 flex-wrap items-center gap-2">{tools}</div>}
      </div>
      {children}
    </div>
  )
}

/**
 * 低频选项的渐进展示；已有非默认值时默认展开，避免把已生效配置藏起来。
 * 传入 fields 后，其中任一字段校验失败都会强制展开，错误不会被折叠隐藏。
 */
export function AdvancedOptions({ children, configuredCount, fields = [], title }: {
  children: ReactNode
  configuredCount: number
  fields?: readonly FieldPath<DeploymentTargetFormValues>[]
  title: string
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(configuredCount > 0)
  const { errors } = useFormState<DeploymentTargetFormValues>({ name: [...fields] })
  const hasError = fields.some(field => Boolean(get(errors, field)))
  const open = expanded || hasError
  return (
    <Collapsible className="grid min-w-0 gap-4" open={open} onOpenChange={setExpanded}>
      <CollapsibleTrigger asChild>
        <Button className="w-fit gap-2 px-2 text-muted-foreground hover:text-foreground" size="sm" type="button" variant="ghost">
          <ChevronDown aria-hidden="true" className={cn('size-4 transition-transform duration-fast motion-reduce:transition-none', open && 'rotate-180')} />
          <span>{title}</span>
          {configuredCount > 0 && <StatusBadge tone="info">{t('deploymentTargetEditor.configuredCount', { count: configuredCount })}</StatusBadge>}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="grid min-w-0 gap-4 border-l-2 border-border pl-4">
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}

/** 布尔配置统一用 Switch，标题与说明都作为可访问名称和描述。 */
export function SwitchField({ description, disabled, label, name, revalidate }: {
  description?: ReactNode
  disabled?: boolean
  label: string
  name: BooleanFieldName
  /** 开关会改变其他字段的显隐或跨字段规则时，提交过一次后需要重新校验这些字段。 */
  revalidate?: readonly FieldPath<DeploymentTargetFormValues>[]
}) {
  const id = useId()
  const { control, formState, trigger } = useFormContext<DeploymentTargetFormValues>()
  return (
    <Controller
      control={control}
      name={name}
      render={({ field }) => (
        <div className="flex min-w-0 items-start justify-between gap-4 rounded-control bg-surface-inset/60 px-4 py-3">
          <div className="min-w-0">
            <label className="text-sm font-medium text-foreground" htmlFor={id}>{label}</label>
            {description && <p className="mt-1 text-xs leading-5 text-muted-foreground" id={`${id}-description`}>{description}</p>}
          </div>
          <Switch
            ref={field.ref}
            aria-describedby={description ? `${id}-description` : undefined}
            checked={field.value}
            className="mt-0.5"
            disabled={disabled}
            id={id}
            onBlur={field.onBlur}
            onCheckedChange={(checked) => {
              field.onChange(checked)
              if (revalidate && formState.isSubmitted)
                void trigger([...revalidate])
            }}
          />
        </div>
      )}
    />
  )
}

/** Kubernetes JSON 片段字段：等宽编辑器 + schema 校验错误。 */
export function JsonField({ hint, label, name, placeholder }: {
  hint?: string
  label: string
  name: TextFieldName
  placeholder?: string
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<DeploymentTargetFormValues>()
  const error = useFieldError(name)
  return (
    <FormField error={error} hint={hint} label={label}>
      {(fieldControl, { labelId }) => (
        <Controller
          control={control}
          name={name}
          render={({ field }) => (
            <CodeEditor
              ariaDescribedBy={fieldControl['aria-describedby']}
              ariaInvalid={Boolean(error)}
              ariaLabelledBy={labelId}
              language="json"
              minHeight="7rem"
              placeholder={placeholder ? t('deploymentTargetEditor.examplePlaceholder', { value: placeholder }) : undefined}
              value={field.value}
              onChange={field.onChange}
            />
          )}
        />
      )}
    </FormField>
  )
}
