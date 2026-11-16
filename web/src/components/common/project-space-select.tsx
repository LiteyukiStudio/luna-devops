import type { Project } from '@/api'
import type { SearchSelectSize } from '@/components/common/search-select'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchMultiSelect, SearchSelect } from '@/components/common/search-select'

type ProjectOption = Pick<Project, 'id' | 'name' | 'identifier' | 'description'>

interface ProjectSpaceControlProps {
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'true' | 'false'
  'className'?: string
  'disabled'?: boolean
  'id'?: string
  'projects': ProjectOption[]
  'size'?: SearchSelectSize
}

export function ProjectSpaceSelect({
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  className,
  disabled,
  id,
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
      className={className}
      disabled={disabled}
      emptyLabel={t('projectSpaces.emptyTitle')}
      id={id}
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
  className,
  disabled,
  id,
  projects,
  size,
  value,
  onChange,
}: ProjectSpaceControlProps & {
  value: string[]
  onChange: (value: string[]) => void
}) {
  const { t } = useTranslation()
  const options = useProjectOptions(projects)
  return (
    <SearchMultiSelect
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid}
      className={className}
      disabled={disabled}
      emptyLabel={t('projectSpaces.emptyTitle')}
      id={id}
      options={options}
      placeholder={t('projectSpaces.selectProjects')}
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
