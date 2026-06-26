import type { Project } from '@/api'
import { Check, ChevronDown } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchSelect } from '@/components/common/search-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

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

export function ProjectSpaceMultiSelect({
  disabled,
  projects,
  value,
  onChange,
}: {
  disabled?: boolean
  projects: Pick<Project, 'id' | 'name' | 'slug' | 'description'>[]
  value: string[]
  onChange: (value: string[]) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const selectedIDs = useMemo(() => new Set(value), [value])
  const selectedProjects = useMemo(
    () => projects.filter(project => selectedIDs.has(project.id)),
    [projects, selectedIDs],
  )
  const options = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return projects
      .filter((project) => {
        if (!keyword)
          return true
        return [project.name, project.slug, project.description].some(text => text.toLowerCase().includes(keyword))
      })
      .slice(0, 50)
  }, [projects, search])
  const label = selectedProjects.length > 0
    ? selectedProjects.map(project => project.name).join(', ')
    : t('projectSpaces.selectProjects')

  function toggleProject(projectID: string) {
    const next = new Set(value)
    if (next.has(projectID))
      next.delete(projectID)
    else
      next.add(projectID)
    onChange(projects.filter(project => next.has(project.id)).map(project => project.id))
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          aria-label={t('projectSpaces.selectProjects')}
          className="h-11 w-full justify-between rounded-2xl px-4 font-normal"
          disabled={disabled}
          type="button"
          variant="outline"
        >
          <span className={cn('min-w-0 truncate text-left', selectedProjects.length === 0 && 'text-muted-foreground')}>
            {label}
          </span>
          <ChevronDown className="ml-2 size-4 shrink-0 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[min(28rem,calc(100vw-2rem))] p-2">
        <div className="grid gap-2">
          <Input
            placeholder={t('projectSpaces.searchProjects')}
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
          <div className="max-h-72 overflow-auto rounded-md border border-border">
            {options.length === 0
              ? <div className="px-3 py-4 text-sm text-muted-foreground">{t('projectSpaces.emptyTitle')}</div>
              : options.map(project => (
                  <button
                    key={project.id}
                    className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-muted"
                    type="button"
                    onClick={() => toggleProject(project.id)}
                  >
                    <span className={cn(
                      'flex size-4 shrink-0 items-center justify-center rounded border border-border',
                      selectedIDs.has(project.id) && 'border-primary bg-primary text-primary-foreground',
                    )}
                    >
                      {selectedIDs.has(project.id) && <Check className="size-3" />}
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate font-medium">{project.name}</span>
                      <span className="block truncate text-xs text-muted-foreground">{project.slug}</span>
                    </span>
                  </button>
                ))}
          </div>
          {selectedProjects.length > 0 && (
            <p className="text-xs text-muted-foreground">
              {t('projectSpaces.selectedProjectCount', { count: selectedProjects.length })}
            </p>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
