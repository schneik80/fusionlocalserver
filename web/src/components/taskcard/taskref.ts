import type { Task } from '../../tasks/types'

// Task refs are the fls:doc sibling (see doccard/docref.ts): the compact,
// text-safe way wiki markdown and chat messages reference a task. The
// stored content carries only a small pseudo-URL token; renderers swap it
// for a rich TaskCard at display time. In the wiki the token travels inside
// a normal markdown link (degrading to a plain named link in any other
// markdown renderer); in chat it sits inline in the plain-text body.

export interface TaskRef {
  hubId: string // GraphQL hub id
  projectId: string // project urn (the task store's key)
  taskId: string // per-project task id, "t<num>"
  title: string // display title at insert time (hydration refreshes it)
}

export const TASK_REF_PREFIX = 'fls:task?'

export function encodeTaskRef(ref: TaskRef): string {
  const sp = new URLSearchParams()
  sp.set('hubId', ref.hubId)
  sp.set('projectId', ref.projectId)
  sp.set('taskId', ref.taskId)
  sp.set('title', ref.title)
  return TASK_REF_PREFIX + sp.toString()
}

export function parseTaskRef(url: string): TaskRef | null {
  if (!url.startsWith(TASK_REF_PREFIX)) return null
  const sp = new URLSearchParams(url.slice(TASK_REF_PREFIX.length))
  const projectId = sp.get('projectId') ?? ''
  const taskId = sp.get('taskId') ?? ''
  if (!projectId || !taskId) return null
  return {
    hubId: sp.get('hubId') ?? '',
    projectId,
    taskId,
    title: sp.get('title') || 'task',
  }
}

export function taskRefFromTask(t: Task): TaskRef {
  return { hubId: t.hubId, projectId: t.projectId, taskId: t.id, title: t.title }
}

// taskRefMarkdown is the wiki-side form: a markdown link whose href is the
// token. Square brackets are stripped from the label only — the token
// itself carries the exact title, percent-encoded.
export function taskRefMarkdown(ref: TaskRef): string {
  return `[${ref.title.replace(/[[\]]/g, '')}](${encodeTaskRef(ref)})`
}
