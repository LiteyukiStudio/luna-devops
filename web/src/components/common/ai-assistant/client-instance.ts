const CLIENT_INSTANCE_STORAGE_KEY = 'luna.ai.client-instance.v1'
const clientInstancePattern = /^[\w-]{16,80}$/

let memoryClientInstanceId: string | undefined

export function readAIClientInstanceId(): string {
  if (memoryClientInstanceId)
    return memoryClientInstanceId
  try {
    const stored = sessionStorage.getItem(CLIENT_INSTANCE_STORAGE_KEY)
    if (stored && clientInstancePattern.test(stored)) {
      memoryClientInstanceId = stored
      return stored
    }
    const created = crypto.randomUUID()
    sessionStorage.setItem(CLIENT_INSTANCE_STORAGE_KEY, created)
    memoryClientInstanceId = created
    return created
  }
  catch {
    memoryClientInstanceId = crypto.randomUUID()
    return memoryClientInstanceId
  }
}
