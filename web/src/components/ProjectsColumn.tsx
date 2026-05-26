import { useProjects } from '../api/queries'
import { useNav } from '../state/nav'
import { usePinToggle } from '../state/pins'
import { Column } from './Column'
import { ItemRow } from './ItemRow'

export function ProjectsColumn() {
  const nav = useNav()
  const projectsQ = useProjects(nav.hubId)
  const { pinnedIds, toggle } = usePinToggle()

  const projects = projectsQ.data ?? []

  return (
    <Column
      title="Projects"
      width={280}
      loading={projectsQ.isLoading}
      error={projectsQ.error as Error | null}
      empty={!projectsQ.isLoading && projects.length === 0}
      emptyText={nav.hubId ? 'No projects in this hub' : 'Select a hub to begin'}
    >
      {projects.map((p) => (
        <ItemRow
          key={p.id}
          item={p}
          selected={nav.project?.id === p.id}
          onClick={() => nav.selectProject(p)}
          pinned={pinnedIds.has(p.id)}
          onTogglePin={toggle}
        />
      ))}
    </Column>
  )
}
