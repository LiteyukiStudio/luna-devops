import type { Project } from '@/api/client'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchSelect } from '@/components/common/search-select'

export function ProjectSpaceSelect({
  disabled,
  projects,
  value,
  onChange,
}: {
  disabled?: boolean
  projects: Pick<Project, 'id' | 'name' | 'slug' | 'description'>[]
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const options = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return projects
      .filter((project) => {
        if (project.id === value)
          return true
        if (!keyword)
          return true
        return [project.name, project.slug, project.description].some(text => text.toLowerCase().includes(keyword))
      })
      .slice(0, 30)
      .map(project => ({
        description: project.slug,
        label: project.name,
        value: project.id,
      }))
  }, [projects, search, value])

  return (
    <div className="w-full sm:w-72">
      <SearchSelect
        disabled={disabled}
        emptyLabel={t('projectSpaces.emptyTitle')}
        maxVisible={30}
        options={options}
        placeholder={t('projectSpaces.selectProject')}
        search={search}
        value={value}
        onSearchChange={setSearch}
        onValueChange={onChange}
      />
    </div>
  )
}
