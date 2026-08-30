import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AIAssistantLauncher } from './launcher'

describe('ai assistant launcher gesture', () => {
  it('opens from pointer click and keyboard activation', async () => {
    const interaction = userEvent.setup()
    const onOpen = vi.fn()
    render(
      <AIAssistantLauncher
        label="打开 AI 助手"
        position={{ x: 24, y: 24 }}
        onOpen={onOpen}
        onPositionChange={vi.fn()}
      />,
    )
    const launcher = screen.getByRole('button', { name: '打开 AI 助手' })

    await interaction.click(launcher)
    expect(onOpen).toHaveBeenCalledOnce()

    launcher.focus()
    await interaction.keyboard('{Enter}')
    expect(onOpen).toHaveBeenCalledTimes(2)
  })

  it('reports a clamped position after dragging', () => {
    const onPositionChange = vi.fn()
    render(
      <AIAssistantLauncher
        label="打开 AI 助手"
        position={{ x: 24, y: 24 }}
        onOpen={vi.fn()}
        onPositionChange={onPositionChange}
      />,
    )
    const launcher = screen.getByRole('button', { name: '打开 AI 助手' })

    fireEvent.pointerDown(launcher, { button: 0, clientX: 24, clientY: 24, isPrimary: true, pointerId: 1 })
    fireEvent.pointerMove(launcher, { buttons: 1, clientX: -120, clientY: -120, isPrimary: true, pointerId: 1 })
    fireEvent.pointerUp(launcher, { button: 0, clientX: -120, clientY: -120, isPrimary: true, pointerId: 1 })

    expect(onPositionChange).toHaveBeenCalledOnce()
    expect(onPositionChange).toHaveBeenCalledWith({ x: 24, y: 24 })
  })

  it('opens from a real touch pointer sequence without waiting for a synthetic click', () => {
    const onOpen = vi.fn()
    render(
      <AIAssistantLauncher
        label="打开 AI 助手"
        position={{ x: 24, y: 24 }}
        onOpen={onOpen}
        onPositionChange={vi.fn()}
      />,
    )
    const launcher = screen.getByRole('button', { name: '打开 AI 助手' })
    fireEvent.pointerDown(launcher, { button: 0, clientX: 120, clientY: 640, isPrimary: true, pointerId: 1, pointerType: 'touch' })
    fireEvent.pointerMove(launcher, { clientX: 124, clientY: 644, isPrimary: true, pointerId: 1, pointerType: 'touch' })
    fireEvent.pointerUp(launcher, { button: 0, clientX: 124, clientY: 644, isPrimary: true, pointerId: 1, pointerType: 'touch' })
    expect(onOpen).toHaveBeenCalledOnce()
  })

  it('does not open when a touch pointer sequence moved beyond the tap threshold', () => {
    const onOpen = vi.fn()
    render(
      <AIAssistantLauncher
        label="打开 AI 助手"
        position={{ x: 24, y: 24 }}
        onOpen={onOpen}
        onPositionChange={vi.fn()}
      />,
    )
    const launcher = screen.getByRole('button', { name: '打开 AI 助手' })
    fireEvent.pointerDown(launcher, { button: 0, clientX: 120, clientY: 640, isPrimary: true, pointerId: 1, pointerType: 'touch' })
    fireEvent.pointerMove(launcher, { clientX: 140, clientY: 650, isPrimary: true, pointerId: 1, pointerType: 'touch' })
    fireEvent.pointerUp(launcher, { button: 0, clientX: 140, clientY: 650, isPrimary: true, pointerId: 1, pointerType: 'touch' })
    expect(onOpen).not.toHaveBeenCalled()
  })

  it('ignores secondary buttons and non-primary pointers', () => {
    const onOpen = vi.fn()
    const onPositionChange = vi.fn()
    render(
      <AIAssistantLauncher
        label="打开 AI 助手"
        position={{ x: 24, y: 24 }}
        onOpen={onOpen}
        onPositionChange={onPositionChange}
      />,
    )
    const launcher = screen.getByRole('button', { name: '打开 AI 助手' })

    fireEvent.pointerDown(launcher, { button: 2, clientX: 24, clientY: 24, isPrimary: true, pointerId: 1 })
    fireEvent.pointerUp(launcher, { button: 2, clientX: 24, clientY: 24, isPrimary: true, pointerId: 1 })
    fireEvent.pointerDown(launcher, { button: 0, clientX: 24, clientY: 24, isPrimary: false, pointerId: 2 })
    fireEvent.pointerUp(launcher, { button: 0, clientX: 24, clientY: 24, isPrimary: false, pointerId: 2 })

    expect(onOpen).not.toHaveBeenCalled()
    expect(onPositionChange).not.toHaveBeenCalled()
  })
})
