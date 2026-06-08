import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, LoadingState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'

export default function UsersPage() {
  const queryClient = useQueryClient()
  const [selectedUserID, setSelectedUserID] = useState<number | null>(null)
  const { data, isFetching } = useQuery({ queryKey: ['admin-users'], queryFn: () => api.users() })
  const users = data ?? []
  const usersLoading = !data && isFetching
  const { data: selectedUserKeys = [], isFetching: isFetchingKeys } = useQuery({
    queryKey: ['admin-user-api-keys', selectedUserID],
    queryFn: () => api.apiKeys(selectedUserID ?? undefined),
    enabled: selectedUserID !== null,
  })
  const update = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Record<string, unknown> }) => api.updateUser(id, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
  })

  return (
    <Panel>
      <SectionHeading
        eyebrow="Identity roster"
        title="Registered users"
        body="Review account state, invite code presence, and operator-level account health without leaving the admin plane."
      />
      {usersLoading ? (
        <LoadingState label="Loading users..." />
      ) : users.length ? (
        <div className="space-y-6">
          <DataTable
            headers={['UID', 'User', 'Invite code', 'Hosted credits', 'Paid user', 'Status', 'Created', 'Action']}
            columns="minmax(0,0.5fr) minmax(0,1.5fr) minmax(0,1fr) minmax(0,0.9fr) minmax(0,0.8fr) minmax(0,0.8fr) minmax(0,1fr) minmax(0,1.5fr)"
            rows={users.map((user) => [
              <code key={`uid-${user.id}`} className="font-mono text-xs text-white">{user.id}</code>,
              <a
                key={`user-${user.id}`}
                href={`/usage-events?user_id=${user.id}`}
                target="_blank"
                rel="noreferrer"
                className="block w-full cursor-pointer rounded-md text-left transition-colors hover:bg-white/5 focus:bg-white/5 focus:outline-none focus:ring-1 focus:ring-primary/40"
                title="View this user's usage events"
                aria-label={`View usage events for ${user.name || user.email}`}
              >
                <div className="font-semibold text-white">{user.name || user.email}</div>
                <div className="mt-1 break-all text-xs text-outline">{user.email}</div>
              </a>,
              <code key={`invite-${user.id}`} className="font-mono text-xs text-white">{user.invite_code || '—'}</code>,
              <span key={`credits-${user.id}`} className="font-mono text-sm text-white">{formatNumber(user.credit_balance ?? 0)}</span>,
              <div key={`paid-${user.id}`} className="flex flex-col gap-1">
                <StatusPill value={user.paid_entitlement ? 'Paid' : 'Unpaid'} />
                {user.paid_entitlement_source ? <span className="text-xs text-outline">{user.paid_entitlement_source}</span> : null}
              </div>,
              <StatusPill key={`status-${user.id}`} value={user.status} />,
              <span key={`created-${user.id}`}>{formatDate(user.created_at)}</span>,
              <div key={`action-${user.id}`} className="flex flex-wrap gap-2">
                <a
                  className="admin-secondary-button"
                  href={`/usage-events?user_id=${user.id}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  View events
                </a>
                <button
                  type="button"
                  className="admin-secondary-button"
                  onClick={() => setSelectedUserID((current) => (current === user.id ? null : user.id))}
                >
                  {selectedUserID === user.id ? 'Hide API keys' : 'View API keys'}
                </button>
                <button
                  type="button"
                  className="admin-secondary-button"
                  disabled={update.isPending}
                  onClick={() => update.mutate({ id: user.id, payload: { paid_entitlement: !user.paid_entitlement } })}
                >
                  {user.paid_entitlement ? 'Mark unpaid' : 'Mark paid'}
                </button>
                <button
                  type="button"
                  className="admin-secondary-button"
                  disabled={update.isPending}
                  onClick={() => update.mutate({ id: user.id, payload: { status: user.status === 'active' ? 'disabled' : 'active' } })}
                >
                  {user.status === 'active' ? 'Disable' : 'Enable'}
                </button>
              </div>,
            ])}
          />

          {selectedUserID !== null ? (
            <div className="admin-card-muted p-5">
              <div className="info-eyebrow text-primary">User API keys</div>
              {isFetchingKeys ? (
                <LoadingState label="Loading user API keys..." className="min-h-24" />
              ) : selectedUserKeys.length ? (
                <div className="mt-4 grid gap-3 lg:grid-cols-2">
                  {selectedUserKeys.map((key) => (
                    <div key={key.id} className="admin-code-card rounded-2xl border border-outline-variant/20 p-4">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div>
                          <div className="font-mono text-sm text-white">{key.key_prefix}</div>
                          <div className="mt-1 text-xs text-outline">{key.plan_name} / {key.allowed_modes || 'external_only'}</div>
                        </div>
                        <StatusPill value={key.status} />
                      </div>
                      <div className="mt-4 flex flex-wrap gap-2 text-xs text-outline">
                        <span className="rounded-full border border-outline-variant/20 px-3 py-1 text-white">External {formatNumber(key.quota_remaining ?? key.quota_total)} / {formatNumber(key.quota_total)}</span>
                        <span className="rounded-full border border-outline-variant/20 px-3 py-1">Used {formatNumber(key.quota_used)}</span>
                        <span className="rounded-full border border-outline-variant/20 px-3 py-1">Hosted credits account-level</span>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="mt-3 text-sm text-outline">No API keys owned by this user.</div>
              )}
            </div>
          ) : null}
        </div>
      ) : (
        <EmptyState title="No users yet" body="User accounts created through company OAuth2 will appear here." />
      )}
    </Panel>
  )
}
