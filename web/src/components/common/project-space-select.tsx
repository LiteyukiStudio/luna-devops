import type { Project } from '@/api'
import type { SearchSelectSize } from '@/components/common/search-select'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchMultiSelect, SearchSelect } from '@/components/common/search-select'

type ProjectOption = Pick<Project, 'id' | 'name' | 'identifier' | 'description'>

interface ProjectSpaceControlProps {
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'true' | 'false'
  'ariaLabel'?: string
  'className'?: string
  'disabled'?: boolean
  'error'?: boolean
  'id'?: string
  'loading'?: boolean
  'projects': ProjectOption[]
  'size'?: SearchSelectSize
}

export function ProjectSpaceSelect({
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  ariaLabel,
  className,
  disabled,
  error,
  id,
  loading,
  projects,
  size,
  value,
  onChange,
}: ProjectSpaceControlProps & {
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const options = useProjectOptions(projects)
  return (
    <SearchSelect
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid}
      ariaLabel={ariaLabel ?? (id ? undefined : t('projectSpaces.selectProject'))}
      className={className}
      disabled={disabled}
      emptyLabel={t('projectSpaces.emptyTitle')}
      error={error}
      id={id}
      loading={loading}
      maxVisible={30}
      options={options}
      placeholder={t('projectSpaces.selectProject')}
      searchPlaceholder={t('projectSpaces.searchProjects')}
      size={size}
      value={value}
      onValueChange={onChange}
    />
  )
}

export function ProjectSpaceMultiSelect({
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  ariaLabel,
  className,
  disabled,
  error,
  id,
  loading,
  placeholder,
  projects,
  pruneUnknownValues,
  size,
  value,
  onChange,
}: ProjectSpaceControlProps & {
  placeholder?: string
  pruneUnknownValues?: boolean
  value: string[]
  onChange: (value: string[]) => void
}) {
  const { t } = useTranslation()
  const options = useProjectOptions(projects)
  return (
    <SearchMultiSelect
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid}
      ariaLabel={ariaLabel ?? (id ? undefined : t('projectSpaces.selectProjects'))}
      className={className}
      disabled={disabled}
      emptyLabel={t('projectSpaces.emptyTitle')}
      error={error}
      id={id}
      loading={loading}
      options={options}
      placeholder={placeholder ?? t('projectSpaces.selectProjects')}
      pruneUnknownValues={pruneUnknownValues}
      searchPlaceholder={t('projectSpaces.searchProjects')}
      selectedLabel={selected => selected.map(project => project.label).join(', ')}
      size={size}
      value={value}
      onValueChange={onChange}
    />
  )
}

function useProjectOptions(projects: ProjectOption[]) {
  return useMemo(() => projects.map(project => ({
    description: project.identifier,
    keywords: project.description,
    label: project.name,
    value: project.id,
  })), [projects])
}
