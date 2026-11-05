import type { ReactNode } from 'react'
import type { FieldPath } from 'react-hook-form'
import type { DeploymentTargetEditorSection, DeploymentTargetFormValues } from './deployment-target-form'
import { useId } from 'react'
import { Controller, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { CodeEditor } from '@/components/common/code-editor'
import { FormField } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { Section } from '@/components/common/section'
import { RadioGroupItem } from '@/components/ui/radio-group'
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
        title={(
          <span className="inline-flex items-center gap-1.5">
            <span id={`${deploymentTargetSectionAnchor(section)}-title`}>{title}</span>
            {description && <HelpTip label={title}>{description}</HelpTip>}
          </span>
        )}
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
          <h3 className="inline-flex items-center gap-1.5 text-sm font-semibold text-foreground">
            {title}
            {description && <HelpTip label={title}>{description}</HelpTip>}
          </h3>
        </div>
        {tools && <div className="flex shrink-0 flex-wrap items-center gap-2">{tools}</div>}
      </div>
      {children}
    </div>
  )
}

/** 单选卡片：标题 + 一行说明，整张卡片是 RadioGroupItem 的 label，保留方向键切换与焦点样式。 */
export function ChoiceCard({ description, icon, id, label, value }: {
  description: string
  icon?: ReactNode
  id: string
  label: string
  value: string
}) {
  return (
    <label
      className="flex min-w-0 cursor-pointer items-start gap-3 rounded-control border border-border bg-background px-3 py-2.5 transition-colors duration-fast hover:bg-surface-subtle has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary-subtle"
      htmlFor={id}
    >
      <RadioGroupItem className="mt-0.5" id={id} value={value} />
      <span className="grid min-w-0 gap-0.5">
        <span className="flex items-center gap-2 text-sm font-medium text-foreground">
          {icon && <span aria-hidden="true" className="text-muted-foreground">{icon}</span>}
          {label}
        </span>
        <span className="text-xs leading-5 text-muted-foreground">{description}</span>
      </span>
    </label>
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
        <div className="flex min-w-0 items-start justify-between gap-4">
          <div className="min-w-0">
            <span className="inline-flex items-center gap-1.5">
              <label className="text-sm font-medium text-foreground" htmlFor={id}>{label}</label>
              {description && <HelpTip id={`${id}-description`} label={label}>{description}</HelpTip>}
            </span>
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
