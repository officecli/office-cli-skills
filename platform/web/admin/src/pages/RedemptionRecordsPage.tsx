import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Input, Select, Space } from 'antd'
import { api } from '../api'
import { DataTable, EmptyState, LoadingState, Panel, SectionHeading, formatDate, formatNumber } from '../components/ui'

type SourceFilter = '' | 'app' | 'cli' | 'tui' | 'desktop'

export default function RedemptionRecordsPage() {
  const [codeFilter, setCodeFilter] = useState('')
  const [userIDFilter, setUserIDFilter] = useState('')
  const [source, setSource] = useState<SourceFilter>('')

  const params = useMemo(() => {
    const p = new URLSearchParams()
    if (codeFilter.trim()) p.set('code', codeFilter.trim())
    if (userIDFilter.trim()) p.set('user_id', userIDFilter.trim())
    if (source) p.set('source', source)
    p.set('limit', '200')
    return p
  }, [codeFilter, userIDFilter, source])

  const { data, isFetching } = useQuery({
    queryKey: ['admin-redemption-records', codeFilter, userIDFilter, source],
    queryFn: () => api.listRedemptionRecords(params),
  })

  const records = data?.items ?? []
  const recordsLoading = !data && isFetching

  return (
    <Panel>
      <SectionHeading
        eyebrow="Redemption records"
        title="Redemption Records"
        body="Review every user redemption event. Filter by code, user ID, or source."
        action={
          <Space wrap>
            <Input.Search placeholder="Filter by code" allowClear value={codeFilter} onSearch={setCodeFilter} onChange={(e) => setCodeFilter(e.target.value)} style={{ width: 200 }} />
            <Input.Search placeholder="Filter by user ID" allowClear value={userIDFilter} onSearch={setUserIDFilter} onChange={(e) => setUserIDFilter(e.target.value)} style={{ width: 160 }} />
            <Select<SourceFilter>
              value={source}
              onChange={setSource}
              style={{ width: 160 }}
              options={[
                { value: '', label: 'All sources' },
                { value: 'app', label: 'Web /app' },
                { value: 'cli', label: 'CLI binary' },
                { value: 'tui', label: 'TUI' },
                { value: 'desktop', label: 'Desktop app' },
              ]}
            />
          </Space>
        }
      />
      {recordsLoading ? (
        <LoadingState label="Loading redemption records..." />
      ) : records.length === 0 ? (
        <EmptyState title="No redemption records" body="User redemption activity appears here after a code is redeemed." />
      ) : (
        <DataTable
          headers={['Time', 'Code', 'User ID', 'Credits', 'Source', 'IP', 'User-Agent']}
          columns="minmax(0,1.1fr) minmax(0,1fr) minmax(0,0.7fr) minmax(0,0.7fr) minmax(0,0.7fr) minmax(0,0.9fr) minmax(0,1.4fr)"
          rows={records.map((r) => [
            <span key={`t-${r.id}`}>{formatDate(r.redeemed_at)}</span>,
            <code key={`c-${r.id}`} className="font-mono text-xs text-white">{r.code}</code>,
            <span key={`u-${r.id}`}>{r.user_id}</span>,
            <span key={`cr-${r.id}`}>{formatNumber(r.credit_amount)}</span>,
            <span key={`s-${r.id}`}>{r.source}</span>,
            <code key={`ip-${r.id}`} className="font-mono text-xs">{r.client_ip || '—'}</code>,
            <span key={`ua-${r.id}`} className="truncate text-xs text-outline" title={r.user_agent}>{r.user_agent || '—'}</span>,
          ])}
        />
      )}
    </Panel>
  )
}
