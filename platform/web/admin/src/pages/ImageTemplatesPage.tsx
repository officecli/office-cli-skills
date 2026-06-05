import { FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, ImageUp, Save, Trash2, XCircle } from 'lucide-react'
import { ApiError, api } from '../api'
import { EmptyState, Panel, SectionHeading, StatusPill } from '../components/ui'
import type { ImagePromptTemplate } from '../types'

const blankTemplate: ImagePromptTemplate = {
  slug: '',
  title: '',
  description: '',
  prompt_preset: '',
  sort_order: 0,
  enabled: true,
}

function normalizeTemplate(template: ImagePromptTemplate): ImagePromptTemplate {
  return {
    ...blankTemplate,
    ...template,
    slug: template.slug.trim(),
    title: template.title.trim(),
    description: template.description.trim(),
    prompt_preset: template.prompt_preset.trim(),
    sort_order: Number(template.sort_order) || 0,
    enabled: Boolean(template.enabled),
  }
}

export default function ImageTemplatesPage() {
  const queryClient = useQueryClient()
  const { data: templates = [], isLoading } = useQuery({ queryKey: ['admin-image-templates'], queryFn: api.imageTemplates })
  const { data: publishRequests = [], isLoading: publishRequestsLoading } = useQuery({ queryKey: ['admin-image-template-publish-requests'], queryFn: api.imageTemplatePublishRequests })
  const [draft, setDraft] = useState<ImagePromptTemplate>(blankTemplate)
  const [editDrafts, setEditDrafts] = useState<Record<number, ImagePromptTemplate>>({})
  const [error, setError] = useState('')
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['admin-image-templates'] })
    queryClient.invalidateQueries({ queryKey: ['admin-image-template-publish-requests'] })
  }

  const sortedTemplates = useMemo(() => [...templates].sort((a, b) => (a.sort_order - b.sort_order) || String(a.title).localeCompare(String(b.title))), [templates])

  const create = useMutation({
    mutationFn: (payload: ImagePromptTemplate) => api.createImageTemplate(normalizeTemplate(payload)),
    onSuccess: async () => {
      setDraft(blankTemplate)
      setError('')
      await invalidate()
    },
    onError: (err) => setError(errorMessage(err, 'Failed to create template.')),
  })
  const update = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: ImagePromptTemplate }) => api.updateImageTemplate(id, normalizeTemplate(payload)),
    onSuccess: async () => {
      setError('')
      await invalidate()
    },
    onError: (err) => setError(errorMessage(err, 'Failed to save template.')),
  })
  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) => enabled ? api.enableImageTemplate(id) : api.disableImageTemplate(id),
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err, 'Failed to update template status.')),
  })
  const remove = useMutation({
    mutationFn: api.deleteImageTemplate,
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err, 'Failed to delete template.')),
  })
  const upload = useMutation({
    mutationFn: ({ id, file }: { id: number; file: File }) => api.uploadImageTemplateThumbnail(id, file),
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err, 'Failed to upload thumbnail.')),
  })
  const review = useMutation({
    mutationFn: ({ id, action }: { id: number; action: 'approve' | 'reject' }) => api.reviewImageTemplatePublishRequest(id, { action }),
    onSuccess: invalidate,
    onError: (err) => setError(errorMessage(err, 'Failed to review publish request.')),
  })

  function submitCreate(event: FormEvent) {
    event.preventDefault()
    create.mutate(draft)
  }

  function draftFor(template: ImagePromptTemplate) {
    if (!template.id) return template
    return editDrafts[template.id] ?? template
  }

  function setTemplateDraft(id: number, next: ImagePromptTemplate) {
    setEditDrafts((current) => ({ ...current, [id]: next }))
  }

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Image generation"
          title="Prompt templates"
          body="Templates are server-managed preset prompts. OfficeDex fetches this list live and passes the selected template id to OfficeCLI generation."
        />
        {error ? <div className="admin-card-muted mb-4 rounded-2xl border border-red-400/40 p-4 text-sm text-red-200">{error}</div> : null}
        <form className="admin-card-muted grid gap-4 p-5 md:grid-cols-2 xl:grid-cols-4" onSubmit={submitCreate}>
          <TextField label="Slug" value={draft.slug} onChange={(value) => setDraft({ ...draft, slug: value })} placeholder="liblib-poster" required />
          <TextField label="Title" value={draft.title} onChange={(value) => setDraft({ ...draft, title: value })} placeholder="Liblib Poster" required />
          <TextField label="Sort order" type="number" value={draft.sort_order} onChange={(value) => setDraft({ ...draft, sort_order: Number(value) })} />
          <label className="flex items-center gap-2 self-end text-sm text-outline"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> Enabled</label>
          <TextField label="Description" className="md:col-span-2" value={draft.description} onChange={(value) => setDraft({ ...draft, description: value })} placeholder="Shown below the card title in OfficeDex." />
          <TextArea label="Prompt preset" className="md:col-span-2" value={draft.prompt_preset} onChange={(value) => setDraft({ ...draft, prompt_preset: value })} placeholder="Preset prompt appended by the server before the user's prompt." required />
          <button type="submit" className="admin-primary-button self-end" disabled={create.isPending}>Create template</button>
        </form>
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Community publishing" title="Publish requests" body="Approve private templates only when the attached image generation task proves the prompt and generated image match." />
        {publishRequestsLoading ? <div className="text-sm text-outline">Loading publish requests...</div> : publishRequests.length ? (
          <div className="grid gap-3">
            {publishRequests.map((request) => (
              <div key={request.id} className="admin-card-muted flex flex-col gap-3 p-5 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <div className="text-sm font-semibold text-white">Request #{request.id} · private template #{request.private_template_id}</div>
                  <div className="mt-1 text-xs text-outline">User #{request.requester_user_id} · generation #{request.provenance_id} · {request.status}</div>
                  {request.submitter_note ? <div className="mt-2 text-sm text-outline-variant">{request.submitter_note}</div> : null}
                </div>
                <div className="flex flex-wrap gap-2">
                  <button type="button" className="admin-primary-button inline-flex items-center gap-2" disabled={review.isPending} onClick={() => review.mutate({ id: request.id, action: 'approve' })}><CheckCircle2 size={14} /> Approve</button>
                  <button type="button" className="admin-secondary-button inline-flex items-center gap-2 text-red-200" disabled={review.isPending} onClick={() => review.mutate({ id: request.id, action: 'reject' })}><XCircle size={14} /> Reject</button>
                </div>
              </div>
            ))}
          </div>
        ) : <EmptyState title="No pending requests" body="User-submitted private templates awaiting review will appear here." />}
      </Panel>

      <Panel>
        <SectionHeading eyebrow="Template library" title="Configured templates" body="Upload a thumbnail for each template so OfficeDex can display visual examples." />
        {isLoading ? <div className="text-sm text-outline">Loading templates...</div> : sortedTemplates.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {sortedTemplates.map((template) => {
              const activeDraft = draftFor(template)
              const id = template.id ?? 0
              return (
                <form key={id || template.slug} className="admin-card-muted grid gap-4 p-5" onSubmit={(event) => {
                  event.preventDefault()
                  id && update.mutate({ id, payload: activeDraft })
                }}>
                  <div className="flex flex-col gap-3 md:flex-row">
                    <div className="h-32 w-full overflow-hidden rounded-2xl border border-outline-variant/20 bg-black/20 md:w-44">
                      {template.thumbnail_url ? <img src={template.thumbnail_url} alt={`${template.title} thumbnail`} className="h-full w-full object-cover" /> : <div className="grid h-full place-items-center text-xs text-outline">No thumbnail</div>}
                    </div>
                    <div className="grid flex-1 gap-3 md:grid-cols-2">
                      <TextField label="Slug" value={activeDraft.slug} onChange={(value) => setTemplateDraft(id, { ...activeDraft, slug: value })} required />
                      <TextField label="Title" value={activeDraft.title} onChange={(value) => setTemplateDraft(id, { ...activeDraft, title: value })} required />
                      <TextField label="Sort order" type="number" value={activeDraft.sort_order} onChange={(value) => setTemplateDraft(id, { ...activeDraft, sort_order: Number(value) })} />
                      <label className="flex items-center gap-2 self-end text-sm text-outline"><input type="checkbox" checked={activeDraft.enabled} onChange={(event) => setTemplateDraft(id, { ...activeDraft, enabled: event.target.checked })} /> Enabled</label>
                    </div>
                  </div>
                  <TextField label="Description" value={activeDraft.description} onChange={(value) => setTemplateDraft(id, { ...activeDraft, description: value })} />
                  <TextArea label="Prompt preset" value={activeDraft.prompt_preset} onChange={(value) => setTemplateDraft(id, { ...activeDraft, prompt_preset: value })} required />
                  <div className="flex flex-wrap items-center gap-3">
                    <StatusPill value={template.enabled ? 'enabled' : 'disabled'} />
                    <label className="admin-secondary-button inline-flex cursor-pointer items-center gap-2">
                      <ImageUp size={14} />
                      Upload thumbnail
                      <input type="file" accept="image/png,image/jpeg,image/webp" className="hidden" onChange={(event) => {
                        const file = event.target.files?.[0]
                        if (id && file) upload.mutate({ id, file })
                        event.currentTarget.value = ''
                      }} />
                    </label>
                    <button type="submit" className="admin-primary-button inline-flex items-center gap-2" disabled={update.isPending}><Save size={14} /> Save</button>
                    <button type="button" className="admin-secondary-button" onClick={() => id && toggle.mutate({ id, enabled: !template.enabled })}>{template.enabled ? 'Disable' : 'Enable'}</button>
                    <button type="button" className="admin-secondary-button inline-flex items-center gap-2 text-red-200" onClick={() => id && window.confirm(`Delete template ${template.title}?`) && remove.mutate(id)}><Trash2 size={14} /> Delete</button>
                  </div>
                </form>
              )
            })}
          </div>
        ) : <EmptyState title="No image templates" body="Create the first template to make it visible in OfficeDex image generation." />}
      </Panel>
    </div>
  )
}

function TextField({ label, value, onChange, type = 'text', placeholder, className = '', required = false }: { label: string; value: string | number; onChange: (value: string) => void; type?: string; placeholder?: string; className?: string; required?: boolean }) {
  return (
    <label className={`text-sm text-outline ${className}`}>{label}
      <input type={type} required={required} className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
    </label>
  )
}

function TextArea({ label, value, onChange, placeholder, className = '', required = false }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string; className?: string; required?: boolean }) {
  return (
    <label className={`text-sm text-outline ${className}`}>{label}
      <textarea required={required} className="admin-code-card mt-2 min-h-28 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
    </label>
  )
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback
}
