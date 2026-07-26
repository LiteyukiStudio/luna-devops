import { CliCommandError } from '../commands/errors.js'

export function normalizeServerOrigin(server: string): string {
  let url: URL
  try {
    url = new URL(server)
  }
  catch (error) {
    throw new CliCommandError(
      'server_url_invalid',
      `Server "${server}" is not a valid absolute URL.`,
      { status: 422, cause: error },
    )
  }

  if (!['http:', 'https:'].includes(url.protocol)) {
    throw new CliCommandError(
      'server_url_invalid',
      'Server URL must use http or https.',
      { status: 422 },
    )
  }
  if (url.username || url.password || url.hash || url.search) {
    throw new CliCommandError(
      'server_url_invalid',
      'Server URL cannot contain credentials, query parameters, or a fragment.',
      { status: 422 },
    )
  }
  if (url.pathname !== '/' && url.pathname !== '') {
    throw new CliCommandError(
      'server_url_subpath_unsupported',
      'Server URL must not contain a path.',
      { status: 422 },
    )
  }

  return url.origin
}
