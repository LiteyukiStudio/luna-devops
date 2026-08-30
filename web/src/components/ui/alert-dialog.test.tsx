import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from './alert-dialog'

describe('alert dialog', () => {
  it('applies the shared button variants to actions and cancellations', () => {
    render(
      <AlertDialog open>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Confirm action</AlertDialogTitle>
            <AlertDialogDescription>This action needs confirmation.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction>Continue</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>,
    )

    expect(screen.getByRole('button', { name: 'Continue' })).toHaveClass('h-9', 'bg-primary')
    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveClass('h-9', 'border-border', 'bg-background')
  })
})
