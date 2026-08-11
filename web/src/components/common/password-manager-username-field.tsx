/** Keeps split credential forms associated with the current password-manager login item. */
export function PasswordManagerUsernameField({ value }: { value?: string }) {
  if (!value)
    return null

  return (
    <input
      aria-hidden="true"
      autoComplete="username"
      className="sr-only"
      name="username"
      readOnly
      tabIndex={-1}
      type="email"
      value={value}
    />
  )
}
