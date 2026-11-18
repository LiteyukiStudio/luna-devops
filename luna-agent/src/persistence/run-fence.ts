import { AsyncLocalStorage } from "node:async_hooks"

export type ActiveRunVersionFence = {
  runId: string
  expectedRunVersion: number
}

const runVersionFenceStorage = new AsyncLocalStorage<ActiveRunVersionFence | null>()

export function withRunningRunVersionFence<T>(
  runId: string,
  expectedRunVersion: number,
  operation: () => Promise<T>,
): Promise<T> {
  return runVersionFenceStorage.run({ runId, expectedRunVersion }, operation)
}

export function withoutRunningRunVersionFence<T>(operation: () => Promise<T>): Promise<T> {
  return runVersionFenceStorage.run(null, operation)
}

export function activeRunningRunFence(): ActiveRunVersionFence | undefined {
  return runVersionFenceStorage.getStore() ?? undefined
}

export function activeRunningRunVersionFence(runId: string): number | undefined {
  const current = runVersionFenceStorage.getStore()
  return current?.runId === runId ? current.expectedRunVersion : undefined
}
