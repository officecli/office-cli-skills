import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Input, Select, Space } from 'antd'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, formatDate, formatNumber } from '../components/ui'

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

  return (
    <Panel>
      <SectionHeading
        eyebrow="Redemption records"
        title="兑换记录"
        body="记录所有用户兑换码使用流水，可按 code、用户 ID、入口来源过滤。"
        action={
          <Space wrap>
            <Input.Search placeholder="按 code" allowClear value={codeFilter} onSearch={setCodeFilter} onChange={(e) => setCodeFilter(e.target.value)} style={{ width: 200 }} />
            <Input.Search placeholder="按用户 ID" allowClear value={userIDFilter} onSearch={setUserIDFilter} onChange={(e) => setUserIDFilter(e.target.value)} style={{ width: 160 }} />
            <Select<SourceFilter>
              value={source}
              onChange={setSource}
              style={{ width: 160 }}
              options={[
                { value: '', label: '全部入口' },
                { value: 'app', label: 'Web /app' },
                { value: 'cli', label: 'CLI 二进制' },
                { value: 'tui', label: 'TUI' },
                { value: 'desktop', label: '桌面 app' },
              ]}
            />
          </Space>
        }
      />
      {records.length === 0 && !isFetching ? (
        <EmptyState title="尚无兑换记录" body="用户兑换兑换码后将在此处显示。" />
      ) : (
        <DataTable
          headers={['时间', 'Code', '用户 ID', 'Credits', '入口', 'IP', 'User-Agent']}
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
