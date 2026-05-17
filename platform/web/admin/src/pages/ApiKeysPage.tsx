import { FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Plus, Save, ToggleLeft, ToggleRight } from 'lucide-react'
import { ApiError, api } from '../api'
import type { ApiKey, User } from '../types'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

type KeyType = 'hosted_only' | 'external_only' | 'hybrid'

type KeyForm = {
  key_type: KeyType
  plan_name: string
  owner_user_id: string
  plan_code: string
  expires_at: string
  quota_total: string
  note: string
}

const blankForm: KeyForm = { key_type: 'hosted_only', plan_name: '', owner_user_id: '', plan_code: '', expires_at: '', quota_total: '', note: '' }
const keyTypePresets: Record<KeyType, { planName: string; allowedModes: string; hostedEnabled: boolean; defaultRuntimeMode: string }> = {
  hosted_only: { planName: 'Hosted Key', allowedModes: 'hosted_only', hostedEnabled: true, defaultRuntimeMode: 'hosted' },
  external_only: { planName: 'External Key', allowedModes: 'external_only', hostedEnabled: false, defaultRuntimeMode: 'external' },
  hybrid: { planName: 'Hybrid Key', allowedModes: 'hybrid', hostedEnabled: true, defaultRuntimeMode: 'hosted' },
}

function formForKeyType(form: KeyForm, keyType: KeyType): KeyForm {
  if (keyType === 'hybrid') {
    return { ...form, key_type: keyType }
  }
  return { ...form, key_type: keyType, quota_total: '' }
}

function keyTypeFromFields(key: Pick<ApiKey, 'allowed_modes' | 'hosted_enabled'>): KeyType {
  if (key.allowed_modes === 'hybrid') return 'hybrid'
  if (key.allowed_modes === 'hosted_only' && key.hosted_enabled) return 'hosted_only'
  return 'external_only'
}

function buildCreatePayload(form: KeyForm) {
  const preset = keyTypePresets[form.key_type]
  const payload: Record<string, unknown> = {
    plan_name: form.plan_name.trim() || preset.planName,
    owner_user_id: form.owner_user_id ? Number(form.owner_user_id) : undefined,
    plan_code: form.plan_code || undefined,
    allowed_modes: preset.allowedModes,
    hosted_enabled: preset.hostedEnabled,
    default_runtime_mode: preset.defaultRuntimeMode,
    expires_at: form.expires_at ? new Date(form.expires_at).toISOString() : undefined,
    note: form.note || undefined,
  }
  if (form.key_type !== 'hosted_only' && form.quota_total !== '') {
    payload.quota_total = Number(form.quota_total)
  }
  return payload
}

function buildUpdatePayload(draft: KeyForm) {
  const preset = keyTypePresets[draft.key_type]
  const payload: Record<string, unknown> = {
    plan_name: draft.plan_name.trim() || preset.planName,
    owner_user_id: draft.owner_user_id ? Number(draft.owner_user_id) : undefined,
    plan_code: draft.plan_code || undefined,
    allowed_modes: preset.allowedModes,
    hosted_enabled: preset.hostedEnabled,
    default_runtime_mode: preset.defaultRuntimeMode,
    expires_at: draft.expires_at ? new Date(draft.expires_at).toISOString() : undefined,
    note: draft.note || undefined,
  }
  if (draft.key_type !== 'hosted_only' && draft.quota_total !== '') {
    payload.quota_total = Number(draft.quota_total)
  }
  return payload
}

function userOptionLabel(user: User) {
  return `UID ${user.id} / ${user.email}${user.name ? ` / ${user.name}` : ''}`
}

export default function ApiKeysPage() {
  const queryClient = useQueryClient()
  const { data: keys = [] } = useQuery({ queryKey: ['admin-api-keys'], queryFn: () => api.apiKeys() })
  const [showCreate, setShowCreate] = useState(false)
  const [showCreateAdvanced, setShowCreateAdvanced] = useState(false)
  const [createForm, setCreateForm] = useState(blankForm)
  const [createUserSearch, setCreateUserSearch] = useState('')
  const [selectedCreateUser, setSelectedCreateUser] = useState<User | null>(null)
  const [createUserError, setCreateUserError] = useState('')
  const [editingId, setEditingId] = useState<number | null>(null)
  const [showEditAdvanced, setShowEditAdvanced] = useState<Record<number, boolean>>({})
  const [revealedKey, setRevealedKey] = useState<string | null>(null)
  const [copyingKeyID, setCopyingKeyID] = useState<number | null>(null)
  const [copiedKeyID, setCopiedKeyID] = useState<number | null>(null)
  const [copyErrorByKey, setCopyErrorByKey] = useState<Record<number, string>>({})
  const createRequiresUser = createForm.key_type !== 'external_only'
  const { data: createUserOptions = [] } = useQuery({
    queryKey: ['admin-users-search', createUserSearch],
    queryFn: () => api.users(createUserSearch.trim()),
    enabled: showCreate && createRequiresUser && createUserSearch.trim().length > 0,
  })

  const create = useMutation({
    mutationFn: (payload: Record<string, unknown>) => api.createApiKey(payload),
    onSuccess: async (result) => {
      setRevealedKey(result.plaintext_key)
      setCreateForm(blankForm)
      setCreateUserSearch('')
      setSelectedCreateUser(null)
      setCreateUserError('')
      setShowCreateAdvanced(false)
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
    key_type: keyTypeFromFields(key),
    plan_name: key.plan_name,
    owner_user_id: String(key.owner_user_id ?? ''),
    plan_code: key.plan_code ?? '',
    expires_at: key.expires_at ? key.expires_at.slice(0, 16) : '',
    quota_total: String(key.quota_total ?? ''),
    note: key.note ?? '',
  }])), [keys])
  const [drafts, setDrafts] = useState<Record<number, KeyForm>>({})
  const activeDraft = (key: ApiKey) => drafts[key.id] ?? initialForms[key.id] ?? {
    key_type: keyTypeFromFields(key),
    plan_name: key.plan_name,
    owner_user_id: String(key.owner_user_id ?? ''),
    plan_code: key.plan_code ?? '',
    expires_at: key.expires_at ? key.expires_at.slice(0, 16) : '',
    quota_total: String(key.quota_total ?? ''),
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
          body="Use this surface for audit-safe credential creation, owner assignment, and status changes across the platform fleet."
          action={<button type="button" className="admin-primary-button" onClick={() => setShowCreate((value) => !value)}><Plus size={16} /> Create API Key</button>}
        />

        {showCreate ? (
          <form className="admin-card-muted mb-6 grid gap-4 p-5 md:grid-cols-3" onSubmit={(event: FormEvent) => {
            event.preventDefault()
            if (createRequiresUser && !createForm.owner_user_id) {
              setCreateUserError('Select a user before creating a hosted access credential.')
              return
            }
            create.mutate(buildCreatePayload(createForm))
          }}>
            <label className="text-sm text-outline">
              Key type
              <select className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.key_type} onChange={(event) => setCreateForm((current) => formForKeyType(current, event.target.value as KeyType))}>
                <option value="hosted_only">Hosted Only</option>
                <option value="external_only">External Only</option>
                <option value="hybrid">Hybrid</option>
              </select>
            </label>
            {createRequiresUser ? (
              <label className="relative text-sm text-outline">
                User
                <input
                  className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40"
                  placeholder="Search uid or email"
                  value={createUserSearch}
                  onChange={(event) => {
                    setCreateUserSearch(event.target.value)
                    setSelectedCreateUser(null)
                    setCreateUserError('')
                    setCreateForm((current) => ({ ...current, owner_user_id: '' }))
                  }}
                />
                {createUserOptions.length ? (
                  <div className="absolute z-10 mt-2 w-full overflow-hidden rounded-2xl border border-outline-variant/20 bg-surface-container-high shadow-xl">
                    {createUserOptions.map((user) => (
                      <button
                        key={user.id}
                        type="button"
                        className="block w-full px-4 py-3 text-left text-xs text-outline hover:bg-surface-container-highest hover:text-white"
                        onClick={() => {
                          setSelectedCreateUser(user)
                          setCreateUserSearch(userOptionLabel(user))
                          setCreateUserError('')
                          setCreateForm((current) => ({ ...current, owner_user_id: String(user.id) }))
                        }}
                      >
                        {userOptionLabel(user)}
                      </button>
                    ))}
                  </div>
                ) : null}
                {selectedCreateUser ? <div className="mt-2 text-xs text-secondary">Selected {userOptionLabel(selectedCreateUser)}</div> : null}
                {createUserError ? <div className="mt-2 text-xs text-rose-200">{createUserError}</div> : null}
              </label>
            ) : null}
            {createForm.key_type !== 'hosted_only' ? (
              <label className="text-sm text-outline">
                External quota
                <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={createForm.quota_total} onChange={(event) => setCreateForm((current) => ({ ...current, quota_total: event.target.value }))} />
              </label>
            ) : null}
            {createForm.key_type !== 'external_only' ? (
              <div className="admin-card-muted rounded-2xl p-4 text-sm text-outline">
                Hosted credentials spend the owner's account hosted credits. Add credits through Billing or account credit ledgers, not on the key.
              </div>
            ) : null}
            <div className="md:col-span-3">
              <button type="button" className="admin-secondary-button" onClick={() => setShowCreateAdvanced((value) => !value)}>{showCreateAdvanced ? 'Hide advanced' : 'Advanced'}</button>
            </div>
            {showCreateAdvanced ? (
              <div className="md:col-span-3 grid gap-4 rounded-2xl border border-outline-variant/20 p-4 md:grid-cols-3">
                <label className="text-sm text-outline">
                  Plan name override
                  <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.plan_name} onChange={(event) => setCreateForm((current) => ({ ...current, plan_name: event.target.value }))} />
                </label>
                <label className="text-sm text-outline">
                  Plan code
                  <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.plan_code} onChange={(event) => setCreateForm((current) => ({ ...current, plan_code: event.target.value }))} />
                </label>
                <label className="text-sm text-outline">
                  Expires at
                  <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="datetime-local" value={createForm.expires_at} onChange={(event) => setCreateForm((current) => ({ ...current, expires_at: event.target.value }))} />
                </label>
                <label className="text-sm text-outline md:col-span-3">
                  Note
                  <textarea className="admin-code-card mt-2 min-h-24 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={createForm.note} onChange={(event) => setCreateForm((current) => ({ ...current, note: event.target.value }))} />
                </label>
              </div>
            ) : null}
            <div className="md:col-span-3 flex gap-3">
              <button type="submit" className="admin-primary-button" disabled={create.isPending || (createRequiresUser && !createForm.owner_user_id)}>Create key</button>
              <button type="button" className="admin-secondary-button" onClick={() => {
                setShowCreate(false)
                setShowCreateAdvanced(false)
                setCreateUserSearch('')
                setSelectedCreateUser(null)
                setCreateUserError('')
              }}>Dismiss</button>
            </div>
          </form>
        ) : null}

        {revealedKey ? (
          <div className="admin-surface-panel mb-6 border border-secondary/20 bg-secondary/10 p-4">
            <div className="info-eyebrow text-secondary">Copy this plaintext key now</div>
            <div className="mt-3 flex flex-wrap items-center gap-3">
              <code className="admin-code-card rounded-2xl px-4 py-3 font-mono text-sm text-white">{revealedKey}</code>
              <button type="button" className="admin-secondary-button" onClick={() => navigator.clipboard?.writeText(revealedKey)}><Copy size={14} /> Copy</button>
            </div>
          </div>
        ) : null}

        {keys.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {keys.map((key) => {
              const draft = activeDraft(key)
              return (
                <div key={key.id} className="admin-card-muted p-5">
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <div className="flex items-center gap-2 text-white"><KeyRound size={16} className="text-primary" /> {key.key_prefix}</div>
                      <div className="mt-2 text-sm text-outline">Created {formatDate(key.created_at)} / last used {formatDate(key.last_used_at)}</div>
                    </div>
                    <StatusPill value={key.status} />
                  </div>

                  <div className="mt-5 flex flex-wrap gap-2 text-xs text-outline">
                    <span className="rounded-full border border-outline-variant/20 px-3 py-1 text-white">{key.allowed_modes || 'external_only'}</span>
                    <span className="rounded-full border border-outline-variant/20 px-3 py-1">default {key.default_runtime_mode || 'external'}</span>
                    <span className="rounded-full border border-outline-variant/20 px-3 py-1">{key.hosted_enabled ? 'hosted enabled' : 'hosted disabled'}</span>
                  </div>

                  <div className="mt-5 grid gap-3 sm:grid-cols-3">
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Plan</div><div className="mt-2 text-white">{key.plan_name}</div></div>
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Owner</div><div className="mt-2 text-white">{key.owner_user_id ?? '—'}</div></div>
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">External used</div><div className="mt-2 text-white">{formatNumber(key.quota_used)}</div></div>
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">External quota</div><div className="mt-2 text-white">{formatNumber(key.quota_remaining ?? key.quota_total)}</div></div>
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Hosted credits</div><div className="mt-2 text-white">Account-level</div></div>
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Plan code</div><div className="mt-2 text-white">{key.plan_code || '—'}</div></div>
                    <div className="admin-code-card rounded-2xl p-4"><div className="info-eyebrow-tight text-outline">Expires</div><div className="mt-2 text-white">{formatDate(key.expires_at)}</div></div>
                  </div>

                  {editingId === key.id ? (
                    <div className="mt-5 grid gap-4">
                      <label className="text-sm text-outline">
                        Edit key type
                        <select className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.key_type} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: formForKeyType(draft, event.target.value as KeyType) }))}>
                          <option value="hosted_only">Hosted Only</option>
                          <option value="external_only">External Only</option>
                          <option value="hybrid">Hybrid</option>
                        </select>
                      </label>
                      {draft.key_type !== 'hosted_only' ? (
                        <label className="text-sm text-outline">
                          External quota
                          <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="0" value={draft.quota_total} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, quota_total: event.target.value } }))} />
                        </label>
                      ) : null}
                      {draft.key_type !== 'external_only' ? (
                        <div className="admin-card-muted rounded-2xl p-4 text-sm text-outline">
                          Hosted credits are account-level; editing this credential cannot grant or remove credits.
                        </div>
                      ) : null}
                      <div>
                        <button type="button" className="admin-secondary-button" onClick={() => setShowEditAdvanced((current) => ({ ...current, [key.id]: !current[key.id] }))}>{showEditAdvanced[key.id] ? 'Hide advanced' : 'Advanced'}</button>
                      </div>
                      {showEditAdvanced[key.id] ? (
                        <div className="grid gap-4 rounded-2xl border border-outline-variant/20 p-4 md:grid-cols-2">
                          <label className="text-sm text-outline">
                            Plan name
                            <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.plan_name} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, plan_name: event.target.value } }))} />
                          </label>
                          <label className="text-sm text-outline">
                            Owner user ID
                            <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="number" min="1" value={draft.owner_user_id} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, owner_user_id: event.target.value } }))} />
                          </label>
                          <label className="text-sm text-outline">
                            Plan code
                            <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.plan_code} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, plan_code: event.target.value } }))} />
                          </label>
                          <label className="text-sm text-outline">
                            Expires at
                            <input className="admin-code-card mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" type="datetime-local" value={draft.expires_at} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, expires_at: event.target.value } }))} />
                          </label>
                          <label className="text-sm text-outline md:col-span-2">
                            Note
                            <textarea className="admin-code-card mt-2 min-h-24 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.note} onChange={(event) => setDrafts((current) => ({ ...current, [key.id]: { ...draft, note: event.target.value } }))} />
                          </label>
                        </div>
                      ) : null}
                    </div>
                  ) : key.note ? <div className="mt-5 text-sm text-outline">{key.note}</div> : null}

                  <div className="mt-5 flex flex-wrap gap-3">
                    <button type="button" className="admin-secondary-button" disabled={!key.plaintext_available || copyingKeyID === key.id} onClick={() => copyStoredKey(key)}>
                      <Copy size={16} />
                      {copyingKeyID === key.id ? 'Copying...' : copiedKeyID === key.id ? 'Copied' : 'Copy full key'}
                    </button>
                    <button type="button" className="admin-secondary-button" onClick={() => update.mutate({ id: key.id, payload: { status: key.status === 'active' ? 'disabled' : 'active' } })}>
                      {key.status === 'active' ? <ToggleLeft size={16} /> : <ToggleRight size={16} />}
                      {key.status === 'active' ? 'Disable key' : 'Enable key'}
                    </button>
                    {editingId === key.id ? (
                      <button type="button" className="admin-primary-button" onClick={() => update.mutate({ id: key.id, payload: buildUpdatePayload(draft) })}>
                        <Save size={16} /> Save changes
                      </button>
                    ) : (
                      <button type="button" className="admin-secondary-button" onClick={() => {
                        setEditingId(key.id)
                        setShowEditAdvanced((current) => ({ ...current, [key.id]: false }))
                      }}>Edit metadata</button>
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
