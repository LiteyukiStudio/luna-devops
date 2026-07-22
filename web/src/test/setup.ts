import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'
import '@testing-library/jest-dom/vitest'

const storageValues = new Map<string, string>()
const testLocalStorage = {
  clear: () => storageValues.clear(),
  getItem: (key: string) => storageValues.get(key) ?? null,
  key: (index: number) => [...storageValues.keys()][index] ?? null,
  get length() {
    return storageValues.size
  },
  removeItem: (key: string) => storageValues.delete(key),
  setItem: (key: string, value: string) => storageValues.set(key, String(value)),
} satisfies Storage

Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: testLocalStorage,
})

class ResizeObserverMock {
  disconnect() {}
  observe() {}
  unobserve() {}
}

Object.defineProperty(globalThis, 'ResizeObserver', {
  configurable: true,
  value: ResizeObserverMock,
})

Object.defineProperty(document, 'elementFromPoint', {
  configurable: true,
  value: () => null,
})

afterEach(() => {
  cleanup()
  testLocalStorage.clear()
})
