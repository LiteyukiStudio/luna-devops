import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AIAssistantLauncher } from './launcher'

function touchList(target: Element, x: number, y: number) {
  return [{ clientX: x, clientY: y, identifier: 1, target }]
}

describe('ai assistant launcher gesture', () => {
  it('opens from a real touch sequence without waiting for a synthetic click', () => {
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
    fireEvent.touchStart(launcher, { touches: touchList(launcher, 120, 640) })
    fireEvent.touchEnd(launcher, { changedTouches: touchList(launcher, 124, 644) })
    expect(onOpen).toHaveBeenCalledOnce()
  })

  it('does not open when a touch sequence moved beyond the tap threshold', () => {
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
    fireEvent.touchStart(launcher, { touches: touchList(launcher, 120, 640) })
    fireEvent.touchEnd(launcher, { changedTouches: touchList(launcher, 140, 650) })
    expect(onOpen).not.toHaveBeenCalled()
  })
})
