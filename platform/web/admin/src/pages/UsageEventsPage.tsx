import { CSSProperties, FormEvent, ReactNode, useEffect, useMemo, useState } from 'react'
import { DndContext, PointerSensor, closestCenter, type DragEndEvent, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, arrayMove, horizontalListSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { Button, DatePicker, Dropdown, Select, Space, Table, Tooltip, Typography } from 'antd'
import type { MenuProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { GripVertical, MoreVertical, RotateCcw, Settings2 } from 'lucide-react'
import { api } from '../api'
import { EmptyState, Panel, SectionHeading, StatusPill, formatDate } from '../components/ui'
import type { AdminPreference, UsageEvent } from '../types'

interface FilterState {
  mode: string[]
  result: string[]
  action: string[]
  reason_code: string
  fingerprint_hash: string
  api_key_id: string
  user_id: string
  client_ip: string
  request_id: string
  start_time: string
  end_time: string
}

type ColumnFixed = 'left' | 'right' | undefined

interface ColumnPreference {
  key: string
  visible: boolean
  fixed?: ColumnFixed
  width: number
}

interface UsageColumn {
  key: string
  label: string
  width: number
  render: (event: UsageEvent) => ReactNode
}

const preferenceKey = 'usage-events-table'
const modeFilterValues = ['free', 'reward', 'paid', 'hosted']
const resultFilterValues = ['allowed', 'blocked']
const actionFilterValues = ['generate', 'status']
const defaultActionFilterValues = actionFilterValues.filter((value) => value !== 'status')

const selectOptions = (values: string[]) => values.map((value) => ({ value, label: value }))
const modeOptions = selectOptions(modeFilterValues)
const resultOptions = selectOptions(resultFilterValues)
const actionOptions = selectOptions(actionFilterValues)

function defaultFilters(): FilterState {
  return {
    mode: [...modeFilterValues],
    result: [...resultFilterValues],
    action: [...defaultActionFilterValues],
    reason_code: '',
    fingerprint_hash: '',
    api_key_id: '',
    user_id: '',
    client_ip: '',
    request_id: '',
    start_time: '',
    end_time: '',
  }
}

export function buildUsageEventParams(filters: FilterState) {
  const params = new URLSearchParams()
  appendValues(params, 'mode', filters.mode)
  appendValues(params, 'result', filters.result)
  appendValues(params, 'action', filters.action)
  if (filters.reason_code.trim()) params.set('reason_code', filters.reason_code.trim())
  if (filters.fingerprint_hash.trim()) params.set('fingerprint_hash', filters.fingerprint_hash.trim())
  if (filters.api_key_id.trim()) params.set('api_key_id', filters.api_key_id.trim())
  if (filters.user_id.trim()) params.set('user_id', filters.user_id.trim())
  if (filters.client_ip.trim()) params.set('client_ip', filters.client_ip.trim())
  if (filters.request_id.trim()) params.set('request_id', filters.request_id.trim())
  if (filters.start_time.trim()) params.set('start_time', filters.start_time.trim())
  if (filters.end_time.trim()) params.set('end_time', filters.end_time.trim())
  return params
}

function appendValues(params: URLSearchParams, key: string, values: string[]) {
  for (const value of values) {
    const trimmed = value.trim()
    if (trimmed) {
      params.append(key, trimmed)
    }
  }
}

function dateTimeValue(value: string): dayjs.Dayjs | null {
  if (!value.trim()) return null
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed : null
}

const usageColumns: UsageColumn[] = [
  { key: 'created_at', label: 'Timestamp', width: 180, render: (event) => compactText(formatDate(event.created_at)) },
  { key: 'mode_result', label: 'Mode / Result', width: 160, render: (event) => <Space size={6}><Typography.Text strong>{event.mode}</Typography.Text><StatusPill value={event.result} /></Space> },
  { key: 'action_reason', label: 'Action / Reason', width: 170, render: (event) => compactText(`${event.action}${event.reason_code ? ` / ${event.reason_code}` : ''}`) },
  { key: 'user_key', label: 'User / Key', width: 150, render: (event) => compactText(`user ${valueOrDash(event.user_id)} / key ${valueOrDash(event.api_key_id)}`) },
  { key: 'fingerprint_hash', label: 'Fingerprint', width: 220, render: (event) => monoText(event.fingerprint_hash) },
  { key: 'client_ip', label: 'Client IP', width: 150, render: (event) => monoText(valueOrDash(event.client_ip)) },
  { key: 'request', label: 'Request', width: 260, render: (event) => monoText(`${valueOrDash(event.request_method)} ${valueOrDash(event.request_host)}${valueOrDash(event.request_path)}`) },
  { key: 'request_id', label: 'Request ID', width: 180, render: (event) => monoText(valueOrDash(event.request_id)) },
  { key: 'cli_document', label: 'CLI / Document', width: 160, render: (event) => compactText(`${valueOrDash(event.cli_version)} / ${valueOrDash(event.document_type)}`) },
  { key: 'runtime_provider', label: 'Runtime / Provider', width: 170, render: (event) => compactText(`${valueOrDash(event.runtime_mode)} / ${valueOrDash(event.provider)}`) },
  { key: 'model_charge', label: 'Model / Charge', width: 180, render: (event) => compactText(`${event.model_name || event.provider || '--'} / ${chargeLabel(event)}`) },
  { key: 'tokens', label: 'Tokens', width: 210, render: renderTokenUsage },
  { key: 'images', label: 'Images', width: 100, render: (event) => compactText(`${event.image_count ?? 0}`) },
  { key: 'credits', label: 'Credits', width: 240, render: (event) => compactText(`reserved ${event.reserved_credits ?? 0} / settled ${event.settled_credits ?? 0} / refund ${event.refund_credits ?? 0}`) },
  { key: 'cost_profit', label: 'Cost / Profit', width: 180, render: (event) => compactText(`${creditsFromMicrousd(event.upstream_cost_microusd)} / ${creditsFromMicrousd(event.profit_microusd)}${event.cap_applied ? ' capped' : ''}`) },
  { key: 'user_agent', label: 'User-Agent', width: 260, render: (event) => monoText(valueOrDash(event.user_agent)) },
  { key: 'forwarded_for', label: 'Forwarded-For', width: 220, render: (event) => monoText(valueOrDash(event.forwarded_for)) },
]

const defaultPreferences: ColumnPreference[] = usageColumns.map((column) => ({
  key: column.key,
  visible: true,
  width: column.width,
}))

const columnByKey = new Map(usageColumns.map((column) => [column.key, column]))

export default function UsageEventsPage() {
  const [searchParams] = useSearchParams()
  const [draft, setDraft] = useState<FilterState>(() => filtersFromSearchParams(searchParams))
  const [filters, setFilters] = useState<FilterState>(() => filtersFromSearchParams(searchParams))
  const [columnPreferences, setColumnPreferences] = useState<ColumnPreference[]>(defaultPreferences)
  const [columnMenuOpen, setColumnMenuOpen] = useState(false)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))

  useEffect(() => {
    const next = (searchParams.get('user_id') ?? '').trim()
    setDraft((current) => (current.user_id === next ? current : { ...current, user_id: next }))
    setFilters((current) => (current.user_id === next ? current : { ...current, user_id: next }))
  }, [searchParams])

  const { data: savedPreferences } = useQuery({
    queryKey: ['admin-preference', preferenceKey],
    queryFn: () => api.adminPreference(preferenceKey),
    retry: false,
  })

  const savePreferences = useMutation({
    mutationFn: (preferences: ColumnPreference[]) => api.saveAdminPreference(preferenceKey, toPreferencePayload(preferences)),
  })

  useEffect(() => {
    setColumnPreferences(mergePreferences(savedPreferences))
  }, [savedPreferences])

  const { data = [] } = useQuery({
    queryKey: ['admin-usage-events', filters],
    queryFn: () => {
      return api.usageEvents(buildUsageEventParams(filters))
    },
  })

  const visiblePreferences = columnPreferences.filter((preference) => preference.visible && columnByKey.has(preference.key))
  const visibleKeys = visiblePreferences.map((preference) => preference.key)
  const tableColumns = useMemo<ColumnsType<UsageEvent>>(() => {
    return visiblePreferences.map((preference) => {
      const definition = columnByKey.get(preference.key)!
      const width = preference.width || definition.width
      return {
        title: (
          <SortableHeader
            columnKey={definition.key}
            label={definition.label}
            fixed={preference.fixed}
            width={width}
            onFixedChange={(fixed) => updateColumn(preference.key, { fixed })}
            onHide={() => updateColumn(preference.key, { visible: false })}
            onWidthChange={(nextWidth, save) => updateColumnWidth(preference.key, nextWidth, save)}
          />
        ),
        key: definition.key,
        dataIndex: definition.key,
        width,
        fixed: preference.fixed,
        render: (_value: unknown, event: UsageEvent) => definition.render(event),
      }
    })
  }, [visiblePreferences])

  const scrollWidth = visiblePreferences.reduce((sum, preference) => sum + (preference.width || columnByKey.get(preference.key)?.width || 160), 0)

  function persist(next: ColumnPreference[]) {
    setColumnPreferences(next)
    savePreferences.mutate(next)
  }

  function updateColumn(key: string, patch: Partial<ColumnPreference>) {
    const next = columnPreferences.map((preference) => (
      preference.key === key ? { ...preference, ...patch, fixed: Object.prototype.hasOwnProperty.call(patch, 'fixed') ? patch.fixed : preference.fixed } : preference
    ))
    persist(next)
  }

  function updateColumnWidth(key: string, width: number, save: boolean) {
    const normalizedWidth = normalizeWidth(width)
    setColumnPreferences((current) => {
      const next = current.map((preference) => (
        preference.key === key ? { ...preference, width: normalizedWidth } : preference
      ))
      if (save) {
        savePreferences.mutate(next)
      }
      return next
    })
  }

  function resetColumns() {
    persist(defaultPreferences)
  }

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = columnPreferences.findIndex((preference) => preference.key === active.id)
    const newIndex = columnPreferences.findIndex((preference) => preference.key === over.id)
    if (oldIndex < 0 || newIndex < 0) return
    persist(arrayMove(columnPreferences, oldIndex, newIndex))
  }

  return (
    <div className="space-y-4">
      <Panel className="admin-usage-events-panel">
        <SectionHeading
          eyebrow="Audit visibility"
          title="Recent usage events"
          body="Filter the event stream to understand why requests were allowed, blocked, or routed through a specific mode."
          action={(
            <Space wrap>
              <Button icon={<Settings2 size={14} />} onClick={() => setColumnMenuOpen((open) => !open)}>Columns</Button>
              <Button icon={<RotateCcw size={14} />} onClick={resetColumns}>Restore defaults</Button>
            </Space>
          )}
        />
        {columnMenuOpen ? (
          <div className="admin-column-menu mb-4" role="menu" aria-label="Usage event columns">
            {usageColumns.map((column) => {
              const preference = columnPreferences.find((item) => item.key === column.key)
              const visible = preference?.visible !== false
              return (
                <button
                  key={column.key}
                  type="button"
                  role="menuitemcheckbox"
                  aria-checked={visible}
                  className="admin-column-menu-item"
                  onClick={() => updateColumn(column.key, { visible: !visible })}
                >
                  <span>{column.label}</span>
                  <span>{visible ? 'Shown' : 'Hidden'}</span>
                </button>
              )
            })}
          </div>
        ) : null}
        <form className="admin-code-card admin-surface-panel admin-usage-filter-bar mb-3 grid gap-3 border border-outline-variant/20 p-4 md:grid-cols-5 xl:grid-cols-11" onSubmit={(event: FormEvent) => {
          event.preventDefault()
          setFilters(draft)
        }}>
          <label className="text-sm text-outline">Mode
            <Select
              mode="multiple"
              aria-label="Mode"
              className="admin-filter-select mt-2 w-full"
              value={draft.mode}
              options={modeOptions}
              maxTagCount="responsive"
              onChange={(value) => setDraft((current) => ({ ...current, mode: value }))}
            />
          </label>
          <label className="text-sm text-outline">Result
            <Select
              mode="multiple"
              aria-label="Result"
              className="admin-filter-select mt-2 w-full"
              value={draft.result}
              options={resultOptions}
              maxTagCount="responsive"
              onChange={(value) => setDraft((current) => ({ ...current, result: value }))}
            />
          </label>
          <label className="text-sm text-outline">Action
            <Select
              mode="multiple"
              aria-label="Action"
              className="admin-filter-select mt-2 w-full"
              value={draft.action}
              options={actionOptions}
              maxTagCount="responsive"
              onChange={(value) => setDraft((current) => ({ ...current, action: value }))}
            />
          </label>
          <label className="text-sm text-outline">Reason code
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.reason_code} onChange={(event) => setDraft((current) => ({ ...current, reason_code: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Fingerprint
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.fingerprint_hash} onChange={(event) => setDraft((current) => ({ ...current, fingerprint_hash: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">API key ID
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.api_key_id} onChange={(event) => setDraft((current) => ({ ...current, api_key_id: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">User ID
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.user_id} onChange={(event) => setDraft((current) => ({ ...current, user_id: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Client IP
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.client_ip} onChange={(event) => setDraft((current) => ({ ...current, client_ip: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Request ID
            <input className="admin-input mt-2 w-full rounded-2xl border border-outline-variant/20 px-4 py-3 text-white outline-none focus:border-primary/40" value={draft.request_id} onChange={(event) => setDraft((current) => ({ ...current, request_id: event.target.value }))} />
          </label>
          <label className="text-sm text-outline">Start time
            <DatePicker
              showTime
              allowClear
              aria-label="Start time"
              className="admin-filter-picker mt-2 w-full"
              format="YYYY-MM-DD HH:mm:ss"
              placeholder="Select start time"
              value={dateTimeValue(draft.start_time)}
              onChange={(value) => setDraft((current) => ({ ...current, start_time: value ? value.toISOString() : '' }))}
            />
          </label>
          <label className="text-sm text-outline">End time
            <DatePicker
              showTime
              allowClear
              aria-label="End time"
              className="admin-filter-picker mt-2 w-full"
              format="YYYY-MM-DD HH:mm:ss"
              placeholder="Select end time"
              value={dateTimeValue(draft.end_time)}
              onChange={(value) => setDraft((current) => ({ ...current, end_time: value ? value.toISOString() : '' }))}
            />
          </label>
          <div className="flex items-end gap-3 md:col-span-5 xl:col-span-11">
            <button type="submit" className="admin-primary-button">Apply filters</button>
            <button type="button" className="admin-secondary-button" onClick={() => {
              const next = defaultFilters()
              setDraft(next)
              setFilters(next)
            }}>Reset</button>
          </div>
        </form>
        {data.length ? (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={visibleKeys} strategy={horizontalListSortingStrategy}>
              <Table
                className="admin-table admin-usage-table"
                columns={tableColumns}
                dataSource={data}
                pagination={false}
                rowKey="id"
                scroll={{ x: Math.max(scrollWidth, 960) }}
                size="small"
              />
            </SortableContext>
          </DndContext>
        ) : (
          <EmptyState title="No usage events matched" body="Adjust the filter set or wait for fresh policy traffic to enter the audit stream." />
        )}
      </Panel>
    </div>
  )
}

function SortableHeader({ columnKey, label, fixed, width, onFixedChange, onHide, onWidthChange }: { columnKey: string; label: string; fixed?: ColumnFixed; width: number; onFixedChange: (fixed: ColumnFixed) => void; onHide: () => void; onWidthChange: (width: number, save: boolean) => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: columnKey })
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.6 : 1,
  }
  const items: MenuProps['items'] = [
    { key: 'left', label: 'Fix left' },
    { key: 'right', label: 'Fix right' },
    { key: 'none', label: 'Unfix' },
    { type: 'divider' },
    { key: 'hide', label: 'Hide column' },
  ]

  return (
    <div ref={setNodeRef} style={style} className="admin-draggable-header">
      <button type="button" className="admin-drag-handle" aria-label="Drag column" title={`Drag ${label}`} {...attributes} {...listeners}>
        <GripVertical size={13} />
      </button>
      <span className="min-w-0 truncate">{label}{fixed ? ` (${fixed})` : ''}</span>
      <Dropdown
        menu={{
          items,
          onClick: ({ key }) => {
            if (key === 'hide') {
              onHide()
              return
            }
            onFixedChange(key === 'left' || key === 'right' ? key : undefined)
          },
        }}
        trigger={['click']}
      >
        <Button className="admin-column-action" size="small" type="text" icon={<MoreVertical size={14} />} aria-label="Column actions" title={`Column actions ${label}`} />
      </Dropdown>
      <button
        type="button"
        className="admin-column-resizer"
        aria-label={`Resize ${label}`}
        title={`Resize ${label}`}
        onMouseDown={(event) => {
          event.preventDefault()
          event.stopPropagation()
          const startX = event.clientX
          const startWidth = width
          const handleMouseMove = (moveEvent: MouseEvent) => {
            onWidthChange(startWidth + moveEvent.clientX - startX, false)
          }
          const handleMouseUp = (upEvent: MouseEvent) => {
            document.removeEventListener('mousemove', handleMouseMove)
            document.removeEventListener('mouseup', handleMouseUp)
            onWidthChange(startWidth + upEvent.clientX - startX, true)
          }
          document.addEventListener('mousemove', handleMouseMove)
          document.addEventListener('mouseup', handleMouseUp)
        }}
      />
    </div>
  )
}

function mergePreferences(saved?: AdminPreference): ColumnPreference[] {
  const savedColumns = Array.isArray(saved?.columns) ? saved.columns : []
  const merged: ColumnPreference[] = []
  for (const savedColumn of savedColumns) {
    if (!columnByKey.has(savedColumn.key)) continue
    merged.push({
      key: savedColumn.key,
      visible: savedColumn.visible !== false,
      fixed: savedColumn.fixed === 'left' || savedColumn.fixed === 'right' ? savedColumn.fixed : undefined,
      width: normalizeWidth(savedColumn.width, columnByKey.get(savedColumn.key)?.width),
    })
  }
  for (const column of usageColumns) {
    if (!merged.some((preference) => preference.key === column.key)) {
      merged.push({ key: column.key, visible: true, width: column.width })
    }
  }
  return merged
}

function toPreferencePayload(columns: ColumnPreference[]): AdminPreference {
  return {
    version: 1,
    columns: columns.map((column) => ({
      key: column.key,
      visible: column.visible,
      fixed: column.fixed,
      width: column.width,
    })),
  }
}

function filtersFromSearchParams(searchParams: URLSearchParams): FilterState {
  const userID = (searchParams.get('user_id') ?? '').trim()
  const filters = defaultFilters()
  return userID ? { ...filters, user_id: userID } : filters
}

function normalizeWidth(width?: number, fallback = 160) {
  if (!Number.isFinite(width)) {
    return fallback
  }
  return Math.min(720, Math.max(96, Math.round(width || fallback)))
}

function compactText(value: string) {
  return <Tooltip title={value}><span className="block max-w-full truncate">{value}</span></Tooltip>
}

function monoText(value: string) {
  return <Tooltip title={value}><span className="admin-table-mono block max-w-full truncate font-mono">{value}</span></Tooltip>
}

function renderTokenUsage(event: UsageEvent) {
  const prompt = event.prompt_tokens ?? 0
  const completion = event.completion_tokens ?? 0
  const reasoning = event.reasoning_tokens ?? 0
  const total = prompt + completion + reasoning
  const detail = `prompt ${formatTokenCount(prompt)} / completion ${formatTokenCount(completion)} / reasoning ${formatTokenCount(reasoning)}`
  const title = `${formatTokenCount(total)} total tokens; ${detail}`
  return (
    <Tooltip title={title}>
      <span className="block max-w-full">
        <span className="block truncate text-white">{formatTokenCount(total)} total tokens</span>
        <span className="block truncate text-xs text-outline">{detail}</span>
      </span>
    </Tooltip>
  )
}

function chargeLabel(event: UsageEvent) {
  if (event.mode === 'hosted') {
    return `${event.settled_credits ?? 0} credits`
  }
  return `${event.billed_units ?? 0} ${event.unit_type ?? ''}`.trim()
}

function valueOrDash(value?: string | number) {
  if (value === undefined || value === null || value === '') {
    return '--'
  }
  return String(value)
}

function creditsFromMicrousd(value?: number) {
  const credits = Math.ceil(((value ?? 0) * 100) / 1_000_000)
  return `${credits} credits`
}

function formatTokenCount(value: number) {
  return Math.max(0, Math.trunc(value)).toLocaleString('en-US')
}
