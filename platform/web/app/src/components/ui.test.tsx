import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StatusPill } from './ui'

describe('StatusPill', () => {
  it('renders successful states with green styling', () => {
    render(<StatusPill value="paid" />)

    expect(screen.getByText('paid')).toHaveClass('text-emerald-300', 'bg-emerald-500/12', 'border-emerald-400/30')
  })

  it('renders pending states with amber styling', () => {
    render(<StatusPill value="pending" />)

    expect(screen.getByText('pending')).toHaveClass('text-amber-200', 'bg-amber-500/12', 'border-amber-400/30')
  })

  it('renders failure states with rose styling', () => {
    render(<StatusPill value="payment_failed" />)

    expect(screen.getByText('payment_failed')).toHaveClass('text-rose-200', 'bg-rose-500/12', 'border-rose-400/30')
  })

  it('renders canceled states with neutral styling', () => {
    render(<StatusPill value="cancelled" />)

    expect(screen.getByText('cancelled')).toHaveClass('text-slate-200', 'bg-slate-500/12', 'border-slate-400/30')
  })

  it('keeps unknown states on the fallback theme', () => {
    render(<StatusPill value="custom_state" />)

    expect(screen.getByText('custom_state')).toHaveClass('text-tertiary', 'bg-tertiary/15', 'border-tertiary/20')
  })
})
