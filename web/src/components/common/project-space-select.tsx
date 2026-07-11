import type { Project } from '@/api'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchMultiSelect, SearchSelect } from '@/components/common/search-select'

type ProjectOption = Pick<Project, 'id' | 'name' | 'slug' | 'description'>

export function ProjectSpaceSelect({ disabled, projects, value, onChange }: {
  disabled?: boolean
  projects: ProjectOption[]
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const options = useProjectOptions(projects)
  return (
    <div className="w-full sm:w-72">
      <SearchSelect
        disabled={disabled}
        emptyLabel={t('projectSpaces.emptyTitle')}
        maxVisible={30}
        options={options}
        placeholder={t('projectSpaces.selectProject')}
        searchPlaceholder={t('projectSpaces.searchProjects')}
        value={value}
        onValueChange={onChange}
      />
    </div>
  )
}

export function ProjectSpaceMultiSelect({ disabled, projects, value, onChange }: {
  disabled?: boolean
  projects: ProjectOption[]
  value: string[]
  onChange: (value: string[]) => void
}) {
  const { t } = useTranslation()
  const options = useProjectOptions(projects)
  return (
    <SearchMultiSelect
      className="h-11 rounded-2xl"
      disabled={disabled}
      emptyLabel={t('projectSpaces.emptyTitle')}
      options={options}
      placeholder={t('projectSpaces.selectProjects')}
      searchPlaceholder={t('projectSpaces.searchProjects')}
      selectedLabel={selected => selected.map(project => project.label).join(', ')}
      value={value}
      onValueChange={onChange}
    />
  )
}

function useProjectOptions(projects: ProjectOption[]) {
  return useMemo(() => projects.map(project => ({
    description: project.slug,
    keywords: project.description,
    label: project.name,
    value: project.id,
  })), [projects])
}
