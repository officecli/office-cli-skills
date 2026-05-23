import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App as AntApp, Button, DatePicker, Drawer, Form, Input, InputNumber, Modal, Select, Space, Switch, Tag, Typography } from 'antd'
import dayjs from 'dayjs'
import { api } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, StatusPill, formatDate, formatNumber } from '../components/ui'
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
      message.success(`兑换码 ${code.code} 已创建（默认禁用，需手动启用）`)
      setCreateOpen(false)
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || '创建失败'),
  })

  const updateMut = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: UpdateRedemptionCodeRequest }) => api.updateRedemptionCode(id, payload),
    onSuccess: async () => {
      message.success('已保存')
      setEditing(null)
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || '保存失败'),
  })

  const toggleMut = useMutation({
    mutationFn: ({ id, enable }: { id: number; enable: boolean }) =>
      enable ? api.enableRedemptionCode(id) : api.disableRedemptionCode(id),
    onSuccess: async () => {
      await invalidate()
    },
    onError: (err: Error) => message.error(err.message || '状态切换失败'),
  })

  const codes = data?.items ?? []

  return (
    <Panel>
      <SectionHeading
        eyebrow="Redemption codes"
        title="兑换码管理"
        body="管理员可创建、启停、修改兑换码。新建的兑换码默认为「禁用」状态，需手动启用方可被用户领取。设置 expires_at 为空即长期有效；max_redemptions 留空即不限总次数。"
        action={
          <Space>
            <Input.Search placeholder="搜索 code" allowClear value={query} onSearch={setQuery} onChange={(e) => setQuery(e.target.value)} style={{ width: 220 }} />
            <Select<StatusFilter>
              value={status}
              onChange={setStatus}
              style={{ width: 140 }}
              options={[
                { value: '', label: '全部状态' },
                { value: 'enabled', label: '已启用' },
                { value: 'disabled', label: '已禁用' },
              ]}
            />
            <Button type="primary" onClick={() => setCreateOpen(true)}>新增兑换码</Button>
          </Space>
        }
      />      {codes.length === 0 && !isFetching ? (
        <EmptyState title="尚无兑换码" body="点击右上角“新增兑换码”创建第一个，新建后默认禁用，待审核确认后再启用。" />
      ) : (
        <DataTable
          headers={['Code', 'Credits', '已用 / 总数', '每人上限', '过期时间', '状态', '操作']}
          columns="minmax(0,1.4fr) minmax(0,0.7fr) minmax(0,1fr) minmax(0,0.7fr) minmax(0,1.2fr) minmax(0,0.8fr) minmax(0,1.4fr)"
          rows={codes.map((code) => [
            <div key={`code-${code.id}`}>
              <code className="font-mono text-sm text-white">{code.code}</code>
              {code.notes ? <div className="mt-1 text-xs text-outline">{code.notes}</div> : null}
            </div>,
            <span key={`credit-${code.id}`} className="text-white">{formatNumber(code.credit_amount)}</span>,
            <span key={`used-${code.id}`}>
              {formatNumber(code.redemptions_used)} / {code.max_redemptions == null ? <Tag color="geekblue">不限</Tag> : formatNumber(code.max_redemptions)}
            </span>,
            <span key={`peruser-${code.id}`}>{code.per_user_limit}</span>,
            <span key={`exp-${code.id}`}>
              {code.expires_at ? formatDate(code.expires_at) : <Tag color="green">长期有效</Tag>}
            </span>,
            <StatusPill key={`status-${code.id}`} value={code.status === 'enabled' ? 'active' : 'disabled'} />,
            <Space key={`action-${code.id}`} wrap>
              <Switch
                checked={code.status === 'enabled'}
                loading={toggleMut.isPending}
                onChange={(checked) => toggleMut.mutate({ id: code.id, enable: checked })}
                checkedChildren="启用"
                unCheckedChildren="禁用"
              />
              <Button size="small" onClick={() => setEditing(code)}>编辑</Button>
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
      title="新增兑换码"
      open={open}
      onClose={onClose}
      width={520}
      destroyOnClose
      extra={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={loading} onClick={() => form.submit()}>创建</Button>
        </Space>
      }
    >
      <Typography.Paragraph type="secondary">新建的兑换码默认为禁用状态，需在列表中手动启用后才能被用户兑换。</Typography.Paragraph>
      <Form<FormValues>
        form={form}
        layout="vertical"
        initialValues={{ credit_amount: 100, per_user_limit: 1, unlimited: false, long_term: false }}
        onFinish={handleFinish}
      >
        <Form.Item label="Code（留空自动生成）" name="code">
          <Input placeholder="例如 PROMO2026, 留空将生成 16 位随机码" />
        </Form.Item>
        <Form.Item
          label="兑换 credits 数量"
          name="credit_amount"
          rules={[{ required: true, message: '请输入正整数' }]}
        >
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="每人可兑换次数" name="per_user_limit" tooltip="同一用户最多可兑换次数（默认 1）">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="总兑换次数不限" name="unlimited" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item shouldUpdate={(prev, curr) => prev.unlimited !== curr.unlimited} noStyle>
          {({ getFieldValue }) => getFieldValue('unlimited') ? null : (
            <Form.Item label="总兑换次数" name="max_redemptions" rules={[{ type: 'number', min: 1, message: '至少 1' }]}>
              <InputNumber min={1} style={{ width: '100%' }} placeholder="例如 1000" />
            </Form.Item>
          )}
        </Form.Item>
        <Form.Item label="长期有效" name="long_term" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item shouldUpdate={(prev, curr) => prev.long_term !== curr.long_term} noStyle>
          {({ getFieldValue }) => getFieldValue('long_term') ? null : (
            <Form.Item label="过期时间" name="expires_at">
              <DatePicker showTime style={{ width: '100%' }} />
            </Form.Item>
          )}
        </Form.Item>
        <Form.Item label="备注" name="notes">
          <Input.TextArea rows={3} placeholder="可选：运营活动名称、用途等" />
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
      title={code ? `编辑兑换码 ${code.code}` : ''}
      onCancel={onClose}
      destroyOnClose
      confirmLoading={loading}
      onOk={() => form.submit()}
      okText="保存"
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
          <Form.Item label="兑换 credits 数量" name="credit_amount" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="每人可兑换次数" name="per_user_limit">
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="总兑换次数不限" name="unlimited" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item shouldUpdate={(prev, curr) => prev.unlimited !== curr.unlimited} noStyle>
            {({ getFieldValue }) => getFieldValue('unlimited') ? null : (
              <Form.Item label="剩余总次数（修改后将作为新的上限）" name="max_redemptions">
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            )}
          </Form.Item>
          <Form.Item label="长期有效" name="long_term" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item shouldUpdate={(prev, curr) => prev.long_term !== curr.long_term} noStyle>
            {({ getFieldValue }) => getFieldValue('long_term') ? null : (
              <Form.Item label="过期时间" name="expires_at">
                <DatePicker showTime style={{ width: '100%' }} />
              </Form.Item>
            )}
          </Form.Item>
          <Form.Item label="备注" name="notes">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      ) : null}
    </Modal>
  )
}
