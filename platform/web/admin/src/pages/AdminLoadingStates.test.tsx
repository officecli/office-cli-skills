import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { App as AntApp } from 'antd'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ApiKeysPage from './ApiKeysPage'
import BillingEventsPage from './BillingEventsPage'
import CreditLedgerPage from './CreditLedgerPage'
import DashboardPage from './DashboardPage'
import ImageTemplatesPage from './ImageTemplatesPage'
import OperationsFunnelPage from './OperationsFunnelPage'
import OrdersPage from './OrdersPage'
import QuotaSourcesPage from './QuotaSourcesPage'
import RedemptionCodesPage from './RedemptionCodesPage'
import RedemptionRecordsPage from './RedemptionRecordsPage'
import UsersPage from './UsersPage'

const fetchMock = vi.fn()

function renderPage(page: React.ReactNode, { router = false, antApp = false } = {}) {
  const content = (
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {router ? <MemoryRouter>{page}</MemoryRouter> : page}
    </QueryClientProvider>
  )
  return render(antApp ? <AntApp>{content}</AntApp> : content)
}

function pendingFetch() {
  fetchMock.mockImplementation(() => new Promise(() => undefined))
  vi.stubGlobal('fetch', fetchMock)
}

describe('admin page loading states', () => {
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('shows a spinner instead of the empty state while users load', () => {
    pendingFetch()

    renderPage(<UsersPage />, { router: true })

    expect(screen.getByRole('status', { name: /loading users/i })).toBeInTheDocument()
    expect(screen.queryByText(/No users yet/i)).not.toBeInTheDocument()
  })

  it('shows a spinner instead of the empty state while API keys load', () => {
    pendingFetch()

    renderPage(<ApiKeysPage />)

    expect(screen.getByRole('status', { name: /loading api keys/i })).toBeInTheDocument()
    expect(screen.queryByText(/No admin-managed keys yet/i)).not.toBeInTheDocument()
  })

  it('shows spinners instead of empty states while billing lists load', () => {
    pendingFetch()

    const { unmount: unmountOrders } = renderPage(<OrdersPage />)
    expect(screen.getByRole('status', { name: /loading orders/i })).toBeInTheDocument()
    expect(screen.queryByText(/No orders yet/i)).not.toBeInTheDocument()
    unmountOrders()

    const { unmount: unmountBilling } = renderPage(<BillingEventsPage />)
    expect(screen.getByRole('status', { name: /loading billing events/i })).toBeInTheDocument()
    expect(screen.queryByText(/No billing events yet/i)).not.toBeInTheDocument()
    unmountBilling()

    renderPage(<CreditLedgerPage />)
    expect(screen.getByRole('status', { name: /loading credit ledger/i })).toBeInTheDocument()
    expect(screen.queryByText(/No credit ledger entries/i)).not.toBeInTheDocument()
  })

  it('shows a spinner instead of quota-source empty states while quota sources load', () => {
    pendingFetch()

    renderPage(<QuotaSourcesPage />)

    expect(screen.getByRole('status', { name: /loading quota sources/i })).toBeInTheDocument()
    expect(screen.queryByText(/No reward grants matched/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/No paid keys matched/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/No hosted credentials matched/i)).not.toBeInTheDocument()
  })

  it('shows spinners while redemption management lists load', () => {
    pendingFetch()

    const { unmount } = renderPage(<RedemptionCodesPage />, { antApp: true })
    expect(screen.getByRole('status', { name: /loading redemption codes/i })).toBeInTheDocument()
    expect(screen.queryByText(/No redemption codes/i)).not.toBeInTheDocument()
    unmount()

    renderPage(<RedemptionRecordsPage />)
    expect(screen.getByRole('status', { name: /loading redemption records/i })).toBeInTheDocument()
    expect(screen.queryByText(/No redemption records/i)).not.toBeInTheDocument()
  })

  it('shows spinners while image template lists load', () => {
    pendingFetch()

    renderPage(<ImageTemplatesPage />)

    expect(screen.getByRole('status', { name: /loading publish requests/i })).toBeInTheDocument()
    expect(screen.getByRole('status', { name: /loading templates/i })).toBeInTheDocument()
    expect(screen.queryByText(/No pending requests/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/No image templates/i)).not.toBeInTheDocument()
  })

  it('shows a spinner while the operations funnel loads', () => {
    pendingFetch()

    renderPage(<OperationsFunnelPage />)

    expect(screen.getByRole('status', { name: /loading operations funnel/i })).toBeInTheDocument()
    expect(screen.queryByText(/Loading funnel window/i)).not.toBeInTheDocument()
  })

  it('shows dashboard spinners instead of initial empty chart and table states', () => {
    pendingFetch()

    renderPage(<DashboardPage />, { router: true })

    expect(screen.getByRole('status', { name: /loading platform overview/i })).toBeInTheDocument()
    expect(screen.getByRole('status', { name: /loading usage trend/i })).toBeInTheDocument()
    expect(screen.getByRole('status', { name: /loading fingerprint quality/i })).toBeInTheDocument()
    expect(screen.queryByText(/No chart data available/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/No fingerprint rows in the current filter/i)).not.toBeInTheDocument()
  })
})
