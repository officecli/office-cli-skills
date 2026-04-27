import { FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Plus, Save, ToggleLeft, ToggleRight } from 'lucide-react'
import { ApiError, api } from '../api'
import { trackEvent } from '../analytics'
import { APP_ANALYTICS_EVENTS } from '../analytics-events'
import type { ApiKey } from '../types'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

const blankForm = { plan_name: '' }

export default function ApiKeysPage() {
  const queryClient = useQueryClient()
  const { data: keys = [] } = useQuery({ queryKey: ['app-api-keys'], queryFn: api.apiKeys })
  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState(blankForm)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [revealedKey, setRevealedKey] = useState<string | null>(null)
  const [copyingKeyID, setCopyingKeyID] = useState<number | null>(null)
  const [copiedKeyID, setCopiedKeyID] = useState<number | null>(null)
  const [copyErrorByKey, setCopyErrorByKey] = useState<Record<number, string>>({})

  const create = useMutation({
    mutationFn: (payload: Record<string, unknown>) => api.createApiKey(payload),
    onSuccess: async (result) => {
      trackEvent(APP_ANALYTICS_EVENTS.apiKeyCreateSuccess, {
        surface: 'app',
        key_id: result.key.id,
        plan_name: result.key.plan_name,
      })
      setRevealedKey(result.plaintext_key)
      setCreateForm(blankForm)
      setShowCreate(false)
      await queryClient.invalidateQueries({ queryKey: ['app-api-keys'] })
    },
  })

  const update = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Record<string, unknown> }) => api.updateApiKey(id, payload),
    onSuccess: async () => {
      setEditingId(null)
      await queryClient.invalidateQueries({ queryKey: ['app-api-keys'] })
    },
  })

  const initialForms = useMemo(() => Object.fromEntries(keys.map((key) => [key.id, { note: key.note ?? '' }])), [keys])
  const [drafts, setDrafts] = useState<Record<number, { note: string }>>({})

  const activeDraft = (key: ApiKey) => drafts[key.id] ?? initialForms[key.id] ?? { note: key.note ?? '' }

  async function copyStoredKey(key: ApiKey) {
    setCopyingKeyID(key.id)
    setCopiedKeyID((current) => (current === key.id ? null : current))
    setCopyErrorByKey((current) => ({ ...current, [key.id]: '' }))
    try {
      const result = await api.getApiKeyPlaintext(key.id)
      await navigator.clipboard?.writeText(result.plaintext_key)
      setCopiedKeyID(key.id)
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to copy key.'
      setCopyErrorByKey((current) => ({ ...current, [key.id]: message }))
    } finally {
      setCopyingKeyID((current) => (current === key.id ? null : current))
    }
  }

  return (
    <div className="space-y-8">
      <Panel>
        <SectionHeading
          eyebrow="Credential control"
          title="Issue and tune API keys"
          body="Provision new keys, inspect their quota runway, and pause any credential that should stop receiving production traffic."
          action={<button type="button" className="tonal-button" onClick={() => setShowCreate((value) => !value)}><Plus size={16} /> Generate API Key</button>}
        />

        {showCreate ? (
          <form className="panel-muted mb-6 grid gap-4 p-5 md:grid-cols-3" onSubmit={(event: FormEvent) => {
            event.preventDefault()
            create.mutate({
              plan_name: createForm.plan_name,
            })
          }}>
            <label className="text-sm text-outline">
              Plan name
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.plan_name} onChange={(event) => setCreateForm((current) => ({ ...current, plan_name: event.target.value }))} required />
            </label>
            <div className="md:col-span-3 rounded-2xl border border-outline-variant/20 bg-surface-container-low/60 p-4 text-sm text-outline">
              New user-created keys start with zero paid quota. Buy more document generations from Billing or ask an operator to adjust quota in the admin console when needed.
            </div>
            <div className="md:col-span-3 flex gap-3">
              <button type="submit" className="tonal-button" disabled={create.isPending}>Create key</button>
              <button type="button" className="ghost-button" onClick={() => setShowCreate(false)}>Dismiss</button>
            </div>
          </form>
        ) : null}

        {revealedKey ? (
          <div className="soft-panel mb-6 border border-secondary/20 bg-secondary/10 p-4">
            <div className="info-eyebrow text-secondary">Copy this plaintext key now</div>
            <div className="mt-3 flex flex-wrap items-center gap-3">
              <code className="surface-console rounded-2xl px-4 py-3 font-mono text-sm text-white">{revealedKey}</code>
              <button type="button" className="ghost-button" onClick={() => navigator.clipboard?.writeText(revealedKey)}><Copy size={14} /> Copy</button>
            </div>
          </div>
        ) : null}

        {keys.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {keys.map((key) => {
              const draft = activeDraft(key)
              return (
                <div key={key.id} className="panel-muted p-5">
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <div className="flex items-center gap-2 text-white"><KeyRound size={16} className="text-primary" /> {key.key_prefix}</div>
                      <div className="mt-2 text-sm text-outline">Created {formatDate(key.created_at)} / last used {formatDate(key.last_used_at)}</div>
                    </div>
                    <StatusPill value={key.status} />
                  </div>

                  <div className="mt-5 grid gap-3 sm:grid-cols-3">
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Plan</div><div className="mt-2 text-white">{key.plan_name}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Used</div><div className="mt-2 text-white">{formatNumber(key.quota_used)}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Remaining</div><div className="mt-2 text-white">{formatNumber(key.quota_remaining)}</div></div>
                  </div>

                  {editingId === key.id ? (
                    <div className="mt-5 grid gap-4">
                      <textarea className="surface-console min-h-24 rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.note} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, note: event.target.value } }))} />
                      <div className="text-xs text-outline">User workspace edits are intentionally limited to note and enable/disable state. Quota, plan, and expiry stay under account/billing control.</div>
                    </div>
                  ) : key.note ? <div className="mt-5 text-sm text-outline">{key.note}</div> : null}

                  <div className="mt-5 flex flex-wrap gap-3">
                    <button type="button" className="ghost-button" disabled={!key.plaintext_available || copyingKeyID === key.id} onClick={() => copyStoredKey(key)}>
                      <Copy size={16} />
                      {copyingKeyID === key.id ? 'Copying...' : copiedKeyID === key.id ? 'Copied' : 'Copy full key'}
                    </button>
                    <button type="button" className="ghost-button" onClick={() => update.mutate({ id: key.id, payload: { status: key.status === 'active' ? 'disabled' : 'active' } })}>
                      {key.status === 'active' ? <ToggleLeft size={16} /> : <ToggleRight size={16} />}
                      {key.status === 'active' ? 'Disable key' : 'Enable key'}
                    </button>
                    {editingId === key.id ? (
                      <button type="button" className="tonal-button" onClick={() => update.mutate({ id: key.id, payload: { note: draft.note || undefined } })}>
                        <Save size={16} /> Save changes
                      </button>
                    ) : (
                      <button type="button" className="ghost-button" onClick={() => setEditingId(key.id)}>Edit metadata</button>
                    )}
                  </div>
                  {!key.plaintext_available ? (
                    <div className="mt-3 text-xs text-outline">This key was created before plaintext retention was added, so the full value cannot be copied again.</div>
                  ) : null}
                  {copyErrorByKey[key.id] ? <div className="mt-3 text-xs text-rose-200">{copyErrorByKey[key.id]}</div> : null}
                </div>
              )
            })}
          </div>
        ) : (
          <EmptyState title="No credentials provisioned" body="Create the first API key for your production workflow and it will appear here immediately." />
        )}
      </Panel>
    </div>
  )
}
