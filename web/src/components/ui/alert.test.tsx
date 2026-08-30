import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Alert, AlertDescription, AlertTitle } from './alert'

describe('alert', () => {
  it('applies the warning semantic surface', () => {
    render(
      <Alert variant="warning">
        <AlertTitle>Low balance</AlertTitle>
        <AlertDescription>Recharge before the next charge.</AlertDescription>
      </Alert>,
    )

    expect(screen.getByRole('alert')).toHaveClass('border-warning-border', 'bg-warning-subtle')
    expect(screen.getByText('Low balance')).toBeVisible()
    expect(screen.getByText('Recharge before the next charge.')).toBeVisible()
  })
})
