export interface RootCommandShortcut {
  readonly name: string
  readonly target: string
  readonly descriptionKey: string
  readonly description: string
}

export const ROOT_COMMAND_SHORTCUTS: readonly RootCommandShortcut[] = Object.freeze([
  {
    name: 'login',
    target: 'auth.login',
    descriptionKey: 'shortcuts.login',
    description: 'Sign in to a Luna DevOps instance.',
  },
  {
    name: 'logout',
    target: 'auth.logout',
    descriptionKey: 'shortcuts.logout',
    description: 'Remove the active Luna credential.',
  },
  {
    name: 'whoami',
    target: 'auth.status',
    descriptionKey: 'shortcuts.whoami',
    description: 'Show the current authentication identity.',
  },
  {
    name: 'doctor',
    target: 'health.doctor',
    descriptionKey: 'shortcuts.doctor',
    description: 'Check the local CLI, authentication, and server compatibility.',
  },
])
