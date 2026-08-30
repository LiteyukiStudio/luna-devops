import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { UserAvatar } from './user-avatar'

class ImageMock extends EventTarget {
  complete = false
  crossOrigin: string | null = null
  naturalWidth = 0
  referrerPolicy = ''
  src = ''
}

let images: ImageMock[] = []

describe('user avatar', () => {
  beforeEach(() => {
    images = []
    vi.stubGlobal('Image', class extends ImageMock {
      constructor() {
        super()
        images.push(this)
      }
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders initials when no image source is available', () => {
    render(<UserAvatar className="size-9" user={{ name: 'Alice' }} />)

    expect(screen.getByText('AL')).toHaveAttribute('data-slot', 'avatar-fallback')
    expect(screen.getByText('AL').parentElement).toHaveAttribute('data-slot', 'avatar')
  })

  it('tries the platform avatar and Gravatar before showing initials', async () => {
    const { container } = render(<UserAvatar user={{ avatarUrl: 'https://example.com/avatar.png', email: 'test@example.com', name: 'Alice' }} />)

    await waitFor(() => expect(images).toHaveLength(1))
    expect(images[0].src).toBe('https://example.com/avatar.png')
    expect(screen.queryByText('AL')).not.toBeInTheDocument()

    act(() => images[0].dispatchEvent(new Event('error')))
    await waitFor(() => expect(images).toHaveLength(2))
    expect(images[1].src).toBe('https://www.gravatar.com/avatar/55502f40dc8b7c769880b10874abc9d0?s=96&d=404')
    expect(screen.queryByText('AL')).not.toBeInTheDocument()

    images[1].complete = true
    images[1].naturalWidth = 96
    act(() => images[1].dispatchEvent(new Event('load')))
    expect(container.querySelector('img')).toHaveAttribute('src', 'https://www.gravatar.com/avatar/55502f40dc8b7c769880b10874abc9d0?s=96&d=404')
  })

  it('shows initials after both remote sources fail', async () => {
    render(<UserAvatar user={{ avatarUrl: 'https://example.com/avatar.png', email: 'test@example.com', name: 'Alice' }} />)

    await waitFor(() => expect(images).toHaveLength(1))
    act(() => images[0].dispatchEvent(new Event('error')))
    await waitFor(() => expect(images).toHaveLength(2))
    act(() => images[1].dispatchEvent(new Event('error')))

    expect(await screen.findByText('AL')).toHaveAttribute('data-slot', 'avatar-fallback')
  })
})
