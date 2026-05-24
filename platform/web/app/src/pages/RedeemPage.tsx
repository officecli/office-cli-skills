import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App as AntApp, Button, Form, Input, Space, Typography } from 'antd'
import { api, ApiError } from '../api'
import { DataTable, EmptyState, Panel, SectionHeading, formatDate, formatNumber } from '../components/ui'
import type { RedeemResponse, RedemptionHistoryItem } from '../types'

const errorMessageMap: Record<string, string> = {
  code_not_found: 'Redemption code not found',
  code_disabled: 'This code has been disabled',
  code_expired: 'This code has expired',
  code_exhausted: 'This code has been fully redeemed',
  code_already_claimed: 'You have already redeemed this code',
  code_required: 'Please enter a redemption code',
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
    return err.message || 'Redemption failed'
  }
  return (err as Error)?.message || 'Redemption failed'
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
      message.success(`Redeemed successfully. +${result.credit_amount} credits. Balance: ${result.new_balance}`)
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
          title="Redemption code"
          body="Enter a redemption code to add credits to your account. Codes may be limited by uses or expiration."
        />
        <Form
          form={form}
          layout="vertical"
          onFinish={({ code }) => redeem.mutate(code)}
          className="max-w-md"
        >
          <Form.Item
            label="Code"
            name="code"
            rules={[{ required: true, message: 'Please enter a code' }, { min: 4, message: 'Code must be at least 4 characters' }]}
          >
            <Input
              placeholder="e.g. PROMO2026"
              autoFocus
              autoComplete="off"
              maxLength={64}
            />
          </Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={redeem.isPending}>Redeem now</Button>
          </Space>
        </Form>
        {lastResult ? (
          <div className="mt-6 rounded-lg border border-emerald-500/40 bg-emerald-500/5 p-4">
            <Typography.Text className="text-emerald-300">Redeemed successfully</Typography.Text>
            <Typography.Paragraph className="!mb-0 !mt-2">
              Code <code>{lastResult.code}</code> added <strong>{formatNumber(lastResult.credit_amount)}</strong> credits. New balance: <strong>{formatNumber(lastResult.new_balance)}</strong>.
            </Typography.Paragraph>
          </div>
        ) : null}
      </Panel>

      <Panel>
        <SectionHeading
          eyebrow="History"
          title="Redemption history"
          body="Account redemption activity, newest first."
        />
        {history.data && history.data.items.length > 0 ? (
          <DataTable
            headers={['Time', 'Code', 'Credits', 'Source', 'IP']}
            rows={history.data.items.map((row: RedemptionHistoryItem) => [
              <span key={`t-${row.id}`}>{formatDate(row.redeemed_at)}</span>,
              <code key={`c-${row.id}`} className="font-mono text-xs">{row.code}</code>,
              <span key={`cr-${row.id}`}>{formatNumber(row.credit_amount)}</span>,
              <span key={`s-${row.id}`}>{row.source}</span>,
              <span key={`ip-${row.id}`} className="text-xs text-outline">{row.client_ip || '—'}</span>,
            ])}
          />
        ) : (
          <EmptyState title="No redemptions yet" body="Successful redemptions will appear here." />
        )}
      </Panel>
    </div>
  )
}
