import { FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Plus, Save, ToggleLeft, ToggleRight } from 'lucide-react'
import { ApiError, api } from '../api'
import type { ApiKey } from '../types'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

const blankForm = { plan_name: '', owner_user_id: '', plan_code: '', allowed_modes: 'external_only', hosted_enabled: false, default_runtime_mode: 'external', expires_at: '', quota_total: '', credit_balance: '', note: '' }

export default function ApiKeysPage() {
  const queryClient = useQueryClient()
  const { data: keys = [] } = useQuery({ queryKey: ['admin-api-keys'], queryFn: api.apiKeys })
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
      setRevealedKey(result.plaintext_key)
      setCreateForm(blankForm)
      setShowCreate(false)
      await queryClient.invalidateQueries({ queryKey: ['admin-api-keys'] })
    },
  })

  const update = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Record<string, unknown> }) => api.updateApiKey(id, payload),
    onSuccess: async () => {
      setEditingId(null)
      await queryClient.invalidateQueries({ queryKey: ['admin-api-keys'] })
    },
  })

  const initialForms = useMemo(() => Object.fromEntries(keys.map((key) => [key.id, {
    plan_name: key.plan_name,
    owner_user_id: String(key.owner_user_id ?? ''),
    plan_code: key.plan_code ?? '',
    allowed_modes: key.allowed_modes ?? 'external_only',
    hosted_enabled: key.hosted_enabled ?? false,
    default_runtime_mode: key.default_runtime_mode ?? 'external',
    expires_at: key.expires_at ? key.expires_at.slice(0, 16) : '',
    quota_total: String(key.quota_total ?? ''),
    quota_used: String(key.quota_used ?? 0),
    credit_balance: String(key.credit_balance ?? 0),
    note: key.note ?? '',
  }])), [keys])
  const [drafts, setDrafts] = useState<Record<number, { plan_name: string; owner_user_id: string; plan_code: string; allowed_modes: string; hosted_enabled: boolean; default_runtime_mode: string; expires_at: string; quota_total: string; quota_used: string; credit_balance: string; note: string }>>({})
  const activeDraft = (key: ApiKey) => drafts[key.id] ?? initialForms[key.id] ?? {
    plan_name: key.plan_name,
    owner_user_id: String(key.owner_user_id ?? ''),
    plan_code: key.plan_code ?? '',
    allowed_modes: key.allowed_modes ?? 'external_only',
    hosted_enabled: key.hosted_enabled ?? false,
    default_runtime_mode: key.default_runtime_mode ?? 'external',
    expires_at: key.expires_at ? key.expires_at.slice(0, 16) : '',
    quota_total: String(key.quota_total ?? ''),
    quota_used: String(key.quota_used ?? 0),
    credit_balance: String(key.credit_balance ?? 0),
    note: key.note ?? '',
  }

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
          eyebrow="Credential governance"
          title="Review and edit platform API keys"
          body="Use this surface for audit-safe key creation, quota corrections, and status changes across the platform fleet."
          action={<button type="button" className="tonal-button" onClick={() => setShowCreate((value) => !value)}><Plus size={16} /> Create API Key</button>}
        />

        {showCreate ? (
          <form className="panel-muted mb-6 grid gap-4 p-5 md:grid-cols-3" onSubmit={(event: FormEvent) => {
            event.preventDefault()
            create.mutate({
              plan_name: createForm.plan_name,
              owner_user_id: createForm.owner_user_id ? Number(createForm.owner_user_id) : undefined,
              plan_code: createForm.plan_code || undefined,
              allowed_modes: createForm.allowed_modes || undefined,
              hosted_enabled: createForm.hosted_enabled,
              default_runtime_mode: createForm.default_runtime_mode || undefined,
              expires_at: createForm.expires_at ? new Date(createForm.expires_at).toISOString() : undefined,
              quota_total: createForm.quota_total ? Number(createForm.quota_total) : undefined,
              credit_balance: createForm.credit_balance ? Number(createForm.credit_balance) : undefined,
              note: createForm.note || undefined,
            })
          }}>
            <label className="text-sm text-outline">
              Plan name
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.plan_name} onChange={(event) => setCreateForm((current) => ({ ...current, plan_name: event.target.value }))} required />
            </label>
            <label className="text-sm text-outline">
              Owner user ID
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="1" value={createForm.owner_user_id} onChange={(event) => setCreateForm((current) => ({ ...current, owner_user_id: event.target.value }))} />
            </label>
            <label className="text-sm text-outline">
              Plan code
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.plan_code} onChange={(event) => setCreateForm((current) => ({ ...current, plan_code: event.target.value }))} />
            </label>
            <label className="text-sm text-outline">
              Quota total
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={createForm.quota_total} onChange={(event) => setCreateForm((current) => ({ ...current, quota_total: event.target.value }))} />
            </label>
            <label className="text-sm text-outline">
              Hosted credits
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={createForm.credit_balance} onChange={(event) => setCreateForm((current) => ({ ...current, credit_balance: event.target.value }))} />
            </label>
            <label className="text-sm text-outline">
              Allowed modes
              <select className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.allowed_modes} onChange={(event) => setCreateForm((current) => ({ ...current, allowed_modes: event.target.value }))}>
                <option value="external_only">external_only</option>
                <option value="hosted_only">hosted_only</option>
                <option value="hybrid">hybrid</option>
              </select>
            </label>
            <label className="text-sm text-outline">
              Default runtime
              <select className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.default_runtime_mode} onChange={(event) => setCreateForm((current) => ({ ...current, default_runtime_mode: event.target.value }))}>
                <option value="external">external</option>
                <option value="hosted">hosted</option>
              </select>
            </label>
            <label className="text-sm text-outline md:col-span-3">
              Expires at
              <input className="surface-console mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="datetime-local" value={createForm.expires_at} onChange={(event) => setCreateForm((current) => ({ ...current, expires_at: event.target.value }))} />
            </label>
            <label className="text-sm text-outline md:col-span-3">
              Note
              <textarea className="surface-console mt-2 min-h-28 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.note} onChange={(event) => setCreateForm((current) => ({ ...current, note: event.target.value }))} />
            </label>
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
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Owner</div><div className="mt-2 text-white">{key.owner_user_id ?? '—'}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Used</div><div className="mt-2 text-white">{formatNumber(key.quota_used)}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Remaining</div><div className="mt-2 text-white">{formatNumber(key.quota_remaining ?? key.quota_total)}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Hosted credits</div><div className="mt-2 text-white">{formatNumber(key.credit_balance)}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Plan code</div><div className="mt-2 text-white">{key.plan_code || '—'}</div></div>
                    <div className="surface-console rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Expires</div><div className="mt-2 text-white">{formatDate(key.expires_at)}</div></div>
                  </div>
                  <div className="mt-3 text-xs text-outline">Modes: {key.allowed_modes || 'external_only'} / default {key.default_runtime_mode || 'external'} / hosted {key.hosted_enabled ? 'enabled' : 'disabled'}</div>

                  {editingId === key.id ? (
                    <div className="mt-5 grid gap-4">
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.plan_name} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, plan_name: event.target.value } }))} />
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="1" value={draft.owner_user_id} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, owner_user_id: event.target.value } }))} />
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.plan_code} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, plan_code: event.target.value } }))} />
                      <select className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.allowed_modes} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, allowed_modes: event.target.value } }))}>
                        <option value="external_only">external_only</option>
                        <option value="hosted_only">hosted_only</option>
                        <option value="hybrid">hybrid</option>
                      </select>
                      <select className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.default_runtime_mode} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, default_runtime_mode: event.target.value } }))}>
                        <option value="external">external</option>
                        <option value="hosted">hosted</option>
                      </select>
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="datetime-local" value={draft.expires_at} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, expires_at: event.target.value } }))} />
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={draft.quota_total} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, quota_total: event.target.value } }))} />
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={draft.quota_used} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, quota_used: event.target.value } }))} />
                      <input className="surface-console rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={draft.credit_balance} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, credit_balance: event.target.value } }))} />
                      <textarea className="surface-console min-h-24 rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.note} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, note: event.target.value } }))} />
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
                      <button type="button" className="tonal-button" onClick={() => update.mutate({ id: key.id, payload: {
                        plan_name: draft.plan_name,
                        owner_user_id: draft.owner_user_id ? Number(draft.owner_user_id) : undefined,
                        plan_code: draft.plan_code || undefined,
                        allowed_modes: draft.allowed_modes || undefined,
                        default_runtime_mode: draft.default_runtime_mode || undefined,
                        expires_at: draft.expires_at ? new Date(draft.expires_at).toISOString() : undefined,
                        quota_total: draft.quota_total ? Number(draft.quota_total) : undefined,
                        quota_used: draft.quota_used ? Number(draft.quota_used) : undefined,
                        credit_balance: draft.credit_balance ? Number(draft.credit_balance) : undefined,
                        note: draft.note || undefined,
                      } })}>
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
          <EmptyState title="No admin-managed keys yet" body="Once a key exists on the platform, it will appear here for governance review." />
        )}
      </Panel>
    </div>
  )
}
