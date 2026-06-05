import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App as AntApp, Button, DatePicker, Drawer, Form, Input, InputNumber, Modal, Select, Space, Switch, Tag, Typography } from 'antd'
import dayjs from 'dayjs'
import { api } from '../api'
import { DataTable, EmptyState, LoadingState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'
import type { CreateRedemptionCodeRequest, RedemptionCode, UpdateRedemptionCodeRequest } from '../types'

type StatusFilter = '' | 'enabled' | 'disabled'

interface FormValues {
  code?: string
  credit_amount: number
  per_user_limit?: number
  max_redemptions?: number | null
  unlimited?: boolean
  expires_at?: dayjs.Dayjs | null
  long_term?: boolean
  notes?: string
}

export default function RedemptionCodesPage() {
  const { message } = AntApp.useApp()
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<StatusFilter>('')
  const [query, setQuery] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<RedemptionCode | null>(null)

  const params = useMemo(() => {
    const search = new URLSearchParams()
    if (status) search.set('status', status)
    if (query.trim()) search.set('q', query.trim())
    search.set('limit', '100')
    return search
  }, [status, query])

  const { data, isFetching } = useQuery({
    queryKey: ['admin-redemption-codes', status, query],
    queryFn: () => api.listRedemptionCodes(params),
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin-redemption-codes'] })

  const createMut = useMutation({
    mutationFn: (payload: CreateRedemptionCodeRequest) => api.createRedemptionCode(payload),
    onSuccess: async (code) => {
      message.success(`Redemption code ${code.code} was created disabled. Enable it when it is ready.`)
      setCreateOpen(false)
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || 'Failed to create redemption code.'),
  })

  const updateMut = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: UpdateRedemptionCodeRequest }) => api.updateRedemptionCode(id, payload),
    onSuccess: async () => {
      message.success('Saved.')
      setEditing(null)
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || 'Failed to save redemption code.'),
  })

  const toggleMut = useMutation({
    mutationFn: ({ id, enable }: { id: number; enable: boolean }) =>
      enable ? api.enableRedemptionCode(id) : api.disableRedemptionCode(id),
    onSuccess: async () => {
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || 'Failed to update status.'),
  })

  const deleteMut = useMutation({
    mutationFn: api.deleteRedemptionCode,
    onSuccess: async () => {
      message.success('Deleted.')
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || 'Failed to delete redemption code.'),
  })

  const codes = data?.items ?? []
  const codesLoading = !data && isFetching

  return (
    <Panel>
      <SectionHeading
        eyebrow="Redemption codes"
        title="Redemption Code Management"
        body="Admins can create, enable, disable, edit, and delete redemption codes. New codes are created disabled and must be enabled before users can redeem them. Leave expires_at empty for no expiry; leave max_redemptions empty for unlimited total claims."
        action={
          <Space>
            <Input.Search placeholder="Search code" allowClear value={query} onSearch={setQuery} onChange={(e) => setQuery(e.target.value)} style={{ width: 220 }} />
            <Select<StatusFilter>
              value={status}
              onChange={setStatus}
              style={{ width: 140 }}
              options={[
                { value: '', label: 'All statuses' },
                { value: 'enabled', label: 'Enabled' },
                { value: 'disabled', label: 'Disabled' },
              ]}
            />
            <Button type="primary" onClick={() => setCreateOpen(true)}>New Code</Button>
          </Space>
        }
      />
      {codesLoading ? (
        <LoadingState label="Loading redemption codes..." />
      ) : codes.length === 0 ? (
        <EmptyState title="No redemption codes" body="Create the first code from the top-right action. New codes stay disabled until reviewed and enabled." />
      ) : (
        <DataTable
          headers={['Code', 'Credits', 'Used / Total', 'Per-user Limit', 'Expires At', 'Status', 'Actions']}
          columns="minmax(0,1.4fr) minmax(0,0.7fr) minmax(0,1fr) minmax(0,0.7fr) minmax(0,1.2fr) minmax(0,0.8fr) minmax(0,1.4fr)"
          rows={codes.map((code) => [
            <div key={`code-${code.id}`}>
              <code className="font-mono text-sm text-white">{code.code}</code>
              {code.notes ? <div className="mt-1 text-xs text-outline">{code.notes}</div> : null}
            </div>,
            <span key={`credit-${code.id}`} className="text-white">{formatNumber(code.credit_amount)}</span>,
            <span key={`used-${code.id}`}>
              {formatNumber(code.redemptions_used)} / {code.max_redemptions == null ? <Tag color="geekblue">Unlimited</Tag> : formatNumber(code.max_redemptions)}
            </span>,
            <span key={`peruser-${code.id}`}>{code.per_user_limit}</span>,
            <span key={`exp-${code.id}`}>
              {code.expires_at ? formatDate(code.expires_at) : <Tag color="green">No expiry</Tag>}
            </span>,
            <StatusPill key={`status-${code.id}`} value={code.status === 'enabled' ? 'active' : 'disabled'} />,
            <Space key={`action-${code.id}`} wrap>
              <Switch
                checked={code.status === 'enabled'}
                loading={toggleMut.isPending}
                onChange={(checked) => toggleMut.mutate({ id: code.id, enable: checked })}
                checkedChildren="On"
                unCheckedChildren="Off"
              />
              <Button size="small" onClick={() => setEditing(code)}>Edit</Button>
              <Button
                size="small"
                danger
                loading={deleteMut.isPending}
                onClick={() => {
                  if (window.confirm(`Delete redemption code ${code.code}? This cannot be undone.`)) {
                    deleteMut.mutate(code.id)
                  }
                }}
              >
                Delete
              </Button>
            </Space>,
          ])}
        />
      )}

      <CreateCodeDrawer
        open={createOpen}
        loading={createMut.isPending}
        onClose={() => setCreateOpen(false)}
        onSubmit={(payload) => createMut.mutate(payload)}
      />

      <EditCodeModal
        code={editing}
        loading={updateMut.isPending}
        onClose={() => setEditing(null)}
        onSubmit={(payload) => editing && updateMut.mutate({ id: editing.id, payload })}
      />
    </Panel>
  )
}

function CreateCodeDrawer({ open, loading, onClose, onSubmit }: {
  open: boolean
  loading: boolean
  onClose: () => void
  onSubmit: (payload: CreateRedemptionCodeRequest) => void
}) {
  const [form] = Form.useForm<FormValues>()

  const handleFinish = (values: FormValues) => {
    const payload: CreateRedemptionCodeRequest = {
      code: values.code?.trim() || undefined,
      credit_amount: values.credit_amount,
      per_user_limit: values.per_user_limit ?? 1,
      max_redemptions: values.unlimited ? null : values.max_redemptions ?? null,
      expires_at: values.long_term ? null : values.expires_at ? values.expires_at.toISOString() : null,
      notes: values.notes?.trim() || undefined,
    }
    onSubmit(payload)
  }

  return (
    <Drawer
      title="New Redemption Code"
      open={open}
      onClose={onClose}
      width={520}
      destroyOnClose
      extra={
        <Space>
          <Button onClick={onClose}>Cancel</Button>
          <Button type="primary" loading={loading} onClick={() => form.submit()}>Create</Button>
        </Space>
      }
    >
      <Typography.Paragraph type="secondary">New codes are disabled by default and must be enabled in the list before users can redeem them.</Typography.Paragraph>
      <Form<FormValues>
        form={form}
        layout="vertical"
        initialValues={{ credit_amount: 100, per_user_limit: 1, unlimited: false, long_term: false }}
        onFinish={handleFinish}
      >
        <Form.Item label="Code (leave blank to auto-generate)" name="code">
          <Input
            placeholder="Example: PROMO2026. Blank generates a random 16-character code."
            onChange={(e) => form.setFieldValue('code', e.target.value.toUpperCase())}
          />
        </Form.Item>
        <Form.Item
          label="Credits granted"
          name="credit_amount"
          rules={[{ required: true, message: 'Enter a positive integer.' }]}
        >
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="Per-user claim limit" name="per_user_limit" tooltip="Maximum claims per user. Defaults to 1.">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="Unlimited total claims" name="unlimited" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item shouldUpdate={(prev, curr) => prev.unlimited !== curr.unlimited} noStyle>
          {({ getFieldValue }) => getFieldValue('unlimited') ? null : (
            <Form.Item label="Total claim limit" name="max_redemptions" rules={[{ type: 'number', min: 1, message: 'Minimum is 1.' }]}>
              <InputNumber min={1} style={{ width: '100%' }} placeholder="Example: 1000" />
            </Form.Item>
          )}
        </Form.Item>
        <Form.Item label="No expiry" name="long_term" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item shouldUpdate={(prev, curr) => prev.long_term !== curr.long_term} noStyle>
          {({ getFieldValue }) => getFieldValue('long_term') ? null : (
            <Form.Item label="Expires at" name="expires_at">
              <DatePicker showTime style={{ width: '100%' }} />
            </Form.Item>
          )}
        </Form.Item>
        <Form.Item label="Notes" name="notes">
          <Input.TextArea rows={3} placeholder="Optional campaign name, purpose, or review notes." />
        </Form.Item>
      </Form>
    </Drawer>
  )
}

function EditCodeModal({ code, loading, onClose, onSubmit }: {
  code: RedemptionCode | null
  loading: boolean
  onClose: () => void
  onSubmit: (payload: UpdateRedemptionCodeRequest) => void
}) {
  const [form] = Form.useForm<FormValues>()

  return (
    <Modal
      open={!!code}
      title={code ? `Edit redemption code ${code.code}` : ''}
      onCancel={onClose}
      destroyOnClose
      confirmLoading={loading}
      onOk={() => form.submit()}
      okText="Save"
    >
      {code ? (
        <Form<FormValues>
          form={form}
          layout="vertical"
          initialValues={{
            credit_amount: code.credit_amount,
            per_user_limit: code.per_user_limit,
            unlimited: code.max_redemptions == null,
            max_redemptions: code.max_redemptions ?? undefined,
            long_term: code.expires_at == null,
            expires_at: code.expires_at ? dayjs(code.expires_at) : null,
            notes: code.notes,
          }}
          onFinish={(values) => {
            const payload: UpdateRedemptionCodeRequest = {
              credit_amount: values.credit_amount,
              per_user_limit: values.per_user_limit,
            }
            if (values.unlimited) {
              payload.clear_max_limit = true
            } else if (values.max_redemptions != null) {
              payload.max_redemptions = values.max_redemptions
            }
            if (values.long_term) {
              payload.clear_expires_at = true
            } else if (values.expires_at) {
              payload.expires_at = values.expires_at.toISOString()
            }
            payload.notes = values.notes ?? ''
            onSubmit(payload)
          }}
        >
          <Form.Item label="Credits granted" name="credit_amount" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="Per-user claim limit" name="per_user_limit">
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="Unlimited total claims" name="unlimited" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item shouldUpdate={(prev, curr) => prev.unlimited !== curr.unlimited} noStyle>
            {({ getFieldValue }) => getFieldValue('unlimited') ? null : (
              <Form.Item label="Total claim limit after update" name="max_redemptions">
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            )}
          </Form.Item>
          <Form.Item label="No expiry" name="long_term" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item shouldUpdate={(prev, curr) => prev.long_term !== curr.long_term} noStyle>
            {({ getFieldValue }) => getFieldValue('long_term') ? null : (
              <Form.Item label="Expires at" name="expires_at">
                <DatePicker showTime style={{ width: '100%' }} />
              </Form.Item>
            )}
          </Form.Item>
          <Form.Item label="Notes" name="notes">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      ) : null}
    </Modal>
  )
}
