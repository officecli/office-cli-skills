import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DataTable, StatusPill } from './ui'

describe('StatusPill', () => {
  it('renders successful states with an AntD success tag', () => {
    render(<StatusPill value="paid" />)

    expect(screen.getByText('paid').closest('.ant-tag')).toHaveClass('ant-tag-success')
  })

  it('renders pending states with an AntD warning tag', () => {
    render(<StatusPill value="pending" />)

    expect(screen.getByText('pending').closest('.ant-tag')).toHaveClass('ant-tag-warning')
  })

  it('renders failure states with an AntD error tag', () => {
    render(<StatusPill value="payment_failed" />)

    expect(screen.getByText('payment_failed').closest('.ant-tag')).toHaveClass('ant-tag-error')
  })

  it('renders canceled states with a neutral AntD tag', () => {
    render(<StatusPill value="cancelled" />)

    expect(screen.getByText('cancelled').closest('.ant-tag')).toHaveClass('ant-tag-default')
  })

  it('keeps unknown states on a neutral AntD tag', () => {
    render(<StatusPill value="custom_state" />)

    expect(screen.getByText('custom_state').closest('.ant-tag')).toHaveClass('ant-tag-default')
  })
})

describe('DataTable', () => {
  it('renders rows through AntD table markup', () => {
    render(<DataTable headers={['Name', 'Status']} rows={[[<span key="name">OfficeCLI</span>, <StatusPill key="status" value="active" />]]} />)

    expect(screen.getByRole('table').closest('.ant-table-content')).toBeInTheDocument()
    expect(screen.getByText('OfficeCLI')).toBeInTheDocument()
  })
})
