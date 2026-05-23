import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App as AntApp, Button, Form, Input, Space, Typography } from 'antd'
import { api, ApiError } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, formatDate, formatNumber } from '../components/ui'
import type { RedeemResponse, RedemptionHistoryItem } from '../types'

const errorMessageMap: Record<string, string> = {
  code_not_found: '兑换码不存在',
  code_disabled: '兑换码已禁用',
  code_expired: '兑换码已过期',
  code_exhausted: '兑换码已被领完',
  code_already_claimed: '您已经兑换过该兑换码',
  code_required: '请填写兑换码',
}

function friendlyError(err: unknown): string {
  if (err instanceof ApiError) {
    try {
      const parsed = JSON.parse(err.message)
      if (parsed?.error?.code && errorMessageMap[parsed.error.code]) {
        return errorMessageMap[parsed.error.code]
      }
    } catch {
      // Fall through.
    }
    for (const [code, label] of Object.entries(errorMessageMap)) {
      if (err.message.includes(code)) return label
    }
    return err.message || '兑换失败'
  }
  return (err as Error)?.message || '兑换失败'
}

export default function RedeemPage() {
  const { message } = AntApp.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<{ code: string }>()
  const [lastResult, setLastResult] = useState<RedeemResponse | null>(null)

  const history = useQuery({ queryKey: ['app-redemption-history'], queryFn: api.myRedemptions })

  const redeem = useMutation({
    mutationFn: (code: string) => api.redeemCode(code),
    onSuccess: async (result) => {
      setLastResult(result)
      message.success(`兑换成功，获得 ${result.credit_amount} credits，当前余额 ${result.new_balance}`)
      form.resetFields()
      await queryClient.invalidateQueries({ queryKey: ['app-redemption-history'] })
      await queryClient.invalidateQueries({ queryKey: ['app-quota-summary'] })
      await queryClient.invalidateQueries({ queryKey: ['app-overview'] })
    },
    onError: (err) => {
      message.error(friendlyError(err))
    },
  })

  return (
    <div className="space-y-6">
      <Panel>
        <SectionHeading
          eyebrow="Redeem code"
          title="兑换码"
          body="输入兑换码即可领取 credits，可用于托管推理。每个兑换码可能有使用次数或有效期限制。"
        />
        <Form
          form={form}
          layout="vertical"
          onFinish={({ code }) => redeem.mutate(code)}
          className="max-w-md"
        >
          <Form.Item
            label="兑换码"
            name="code"
            rules={[{ required: true, message: '请输入兑换码' }, { min: 4, message: '兑换码至少 4 位' }]}
          >
            <Input
              placeholder="例如 PROMO2026"
              autoFocus
              autoComplete="off"
              maxLength={64}
              onChange={(e) => form.setFieldValue('code', e.target.value.toUpperCase())}
            />
          </Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={redeem.isPending}>立即兑换</Button>
          </Space>
        </Form>
        {lastResult ? (
          <div className="mt-6 rounded-lg border border-emerald-500/40 bg-emerald-500/5 p-4">
            <Typography.Text className="text-emerald-300">兑换成功</Typography.Text>
            <Typography.Paragraph className="!mb-0 !mt-2">
              兑换码 <code>{lastResult.code}</code> 已为您增加 <strong>{formatNumber(lastResult.credit_amount)}</strong> credits，当前可用余额：<strong>{formatNumber(lastResult.new_balance)}</strong>。
            </Typography.Paragraph>
          </div>
        ) : null}
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="History"
          title="兑换记录"
          body="您账号下的兑换流水，按时间倒序排列。"
        />
        {history.data && history.data.items.length > 0 ? (
          <DataTable
            headers={['时间', 'Code', 'Credits', '入口', 'IP']}
            rows={history.data.items.map((row: RedemptionHistoryItem) => [
              <span key={`t-${row.id}`}>{formatDate(row.redeemed_at)}</span>,
              <code key={`c-${row.id}`} className="font-mono text-xs">{row.code}</code>,
              <span key={`cr-${row.id}`}>{formatNumber(row.credit_amount)}</span>,
              <span key={`s-${row.id}`}>{row.source}</span>,
              <span key={`ip-${row.id}`} className="text-xs text-outline">{row.client_ip || '—'}</span>,
            ])}
          />
        ) : (
          <EmptyState title="暂无兑换记录" body="兑换成功后将在此处显示。" />
        )}
      </Panel>
    </div>
  )
}
