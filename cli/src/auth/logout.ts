import type { ConfigPort } from '../commands/types.js'
import type { LogoutLocalResult } from './types.js'
import { updateConfig } from '../config/store.js'

export async function logoutLocal(
  store: ConfigPort,
): Promise<LogoutLocalResult> {
  let result: LogoutLocalResult = { server: '', loggedOut: false }
  await updateConfig(store, (config) => {
    result = {
      server: config.server,
      loggedOut: config.credential !== null && config.credential !== undefined,
    }
    config.credential = null
    config.project = null
  })
  return result
}
