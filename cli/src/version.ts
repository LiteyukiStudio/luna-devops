declare const __LUNA_CLI_VERSION__: string | undefined

export const CLI_DEVELOPMENT_VERSION = '0.0.0-development'

export const CLI_VERSION
  = typeof __LUNA_CLI_VERSION__ === 'string'
    ? __LUNA_CLI_VERSION__
    : CLI_DEVELOPMENT_VERSION
