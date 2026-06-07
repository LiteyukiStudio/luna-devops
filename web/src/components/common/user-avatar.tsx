import { useState } from 'react'
import { cn } from '@/lib/utils'

interface AvatarUser {
  avatarUrl?: string
  email?: string
  name?: string
}

interface UserAvatarProps {
  className?: string
  user?: AvatarUser
}

const md5S = [
  7,
  12,
  17,
  22,
  7,
  12,
  17,
  22,
  7,
  12,
  17,
  22,
  7,
  12,
  17,
  22,
  5,
  9,
  14,
  20,
  5,
  9,
  14,
  20,
  5,
  9,
  14,
  20,
  5,
  9,
  14,
  20,
  4,
  11,
  16,
  23,
  4,
  11,
  16,
  23,
  4,
  11,
  16,
  23,
  4,
  11,
  16,
  23,
  6,
  10,
  15,
  21,
  6,
  10,
  15,
  21,
  6,
  10,
  15,
  21,
  6,
  10,
  15,
  21,
]

const md5K = Array.from({ length: 64 }, (_, index) => Math.floor(Math.abs(Math.sin(index + 1)) * 0x100000000))

/**
 * 用户头像组件。
 * 用于侧边栏、用户列表、成员列表等需要头像的位置，按平台头像、Gravatar、姓名/邮箱首字母依次兜底；非用户实体不要复用它。
 */
export function UserAvatar({ className, user }: UserAvatarProps) {
  const platformAvatarUrl = user?.avatarUrl?.trim() ?? ''
  const gravatarUrl = gravatarAvatarUrl(user?.email)
  const sourceKey = `${platformAvatarUrl}|${gravatarUrl}`

  return (
    <AvatarImage
      key={sourceKey}
      className={className}
      fallback={initialsFromUser(user)}
      sources={[platformAvatarUrl, gravatarUrl].filter(Boolean)}
    />
  )
}

function AvatarImage({
  className,
  fallback,
  sources,
}: {
  className?: string
  fallback: string
  sources: string[]
}) {
  const [sourceIndex, setSourceIndex] = useState(0)
  const source = sources[sourceIndex]

  return (
    <span
      className={cn(
        'flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary/10 text-sm font-semibold text-primary',
        className,
      )}
    >
      {source
        ? (
            <img
              alt=""
              className="size-full object-cover"
              src={source}
              onError={() => setSourceIndex(current => current + 1)}
            />
          )
        : fallback}
    </span>
  )
}

function initialsFromUser(user?: AvatarUser) {
  const source = user?.name || user?.email || 'U'
  return source.trim().slice(0, 2).toUpperCase()
}

function gravatarAvatarUrl(email?: string) {
  const normalizedEmail = email?.trim().toLowerCase()
  if (!normalizedEmail)
    return ''

  return `https://www.gravatar.com/avatar/${md5(normalizedEmail)}?s=96&d=404`
}

function md5(input: string) {
  const message = new TextEncoder().encode(input)
  const paddedLength = (((message.length + 8) >>> 6) + 1) << 6
  const padded = new Uint8Array(paddedLength)
  padded.set(message)
  padded[message.length] = 0x80

  const view = new DataView(padded.buffer)
  view.setUint32(paddedLength - 8, message.length * 8, true)
  view.setUint32(paddedLength - 4, Math.floor((message.length * 8) / 0x100000000), true)

  let a0 = 0x67452301
  let b0 = 0xEFCDAB89
  let c0 = 0x98BADCFE
  let d0 = 0x10325476

  for (let offset = 0; offset < paddedLength; offset += 64) {
    let a = a0
    let b = b0
    let c = c0
    let d = d0

    for (let i = 0; i < 64; i++) {
      let f = 0
      let g = 0

      if (i < 16) {
        f = (b & c) | (~b & d)
        g = i
      }
      else if (i < 32) {
        f = (d & b) | (~d & c)
        g = (5 * i + 1) % 16
      }
      else if (i < 48) {
        f = b ^ c ^ d
        g = (3 * i + 5) % 16
      }
      else {
        f = c ^ (b | ~d)
        g = (7 * i) % 16
      }

      const next = d
      d = c
      c = b
      b = add32(b, leftRotate(add32(add32(a, f), add32(md5K[i], view.getUint32(offset + g * 4, true))), md5S[i]))
      a = next
    }

    a0 = add32(a0, a)
    b0 = add32(b0, b)
    c0 = add32(c0, c)
    d0 = add32(d0, d)
  }

  return [a0, b0, c0, d0].map(wordToHex).join('')
}

function add32(a: number, b: number) {
  return (a + b) >>> 0
}

function leftRotate(value: number, amount: number) {
  return (value << amount) | (value >>> (32 - amount))
}

function wordToHex(word: number) {
  return [0, 8, 16, 24]
    .map(shift => ((word >>> shift) & 0xFF).toString(16).padStart(2, '0'))
    .join('')
}
