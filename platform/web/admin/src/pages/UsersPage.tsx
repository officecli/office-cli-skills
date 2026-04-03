import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'

export default function UsersPage() {
  const queryClient = useQueryClient()
  const { data: users = [] } = useQuery({ queryKey: ['admin-users'], queryFn: api.users })
  const update = useMutation({
    mutationFn: ({ id, status }: { id: number; status: 'active' | 'disabled' }) => api.updateUser(id, { status }),
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
      {users.length ? (
        <DataTable
          headers={['User', 'Invite code', 'Status', 'Created', 'Action']}
          columns="minmax(0,1.5fr) minmax(0,1fr) minmax(0,0.8fr) minmax(0,1fr) minmax(0,1fr)"
          rows={users.map((user) => [
            <div key={`user-${user.id}`}>
              <div className="font-semibold text-white">{user.name || user.email}</div>
              <div className="mt-1 break-all text-xs text-outline">{user.email}</div>
            </div>,
            <code key={`invite-${user.id}`} className="font-mono text-xs text-white">{user.invite_code || '—'}</code>,
            <StatusPill key={`status-${user.id}`} value={user.status} />,
            <span key={`created-${user.id}`}>{formatDate(user.created_at)}</span>,
            <button
              key={`action-${user.id}`}
              type="button"
              className="ghost-button"
              disabled={update.isPending}
              onClick={() => update.mutate({ id: user.id, status: user.status === 'active' ? 'disabled' : 'active' })}
            >
              {user.status === 'active' ? 'Disable' : 'Enable'}
            </button>,
          ])}
        />
      ) : (
        <EmptyState title="No users yet" body="User accounts created through Google OAuth will appear here." />
      )}
    </Panel>
  )
}
