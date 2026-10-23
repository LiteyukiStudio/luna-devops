# Configuration and Secrets

A deployment can combine environment variables (plain or encrypted), shared configuration, configuration files, and secret files.

## Environment variables

Manage them in the environment variable table under **Runtime → Environment and configuration** in the deploy config editor. Each row has a name, a value, and an **Encrypted** switch:

- **Plain variables** are stored in the deployment-managed ConfigMap and injected as environment variables during release. Authorized clients may read them, so **do not place passwords, tokens, or private keys here**.
- **Encrypted variables** are stored encrypted when the switch is on. After saving, only `••••••` is shown and the value is never returned. Use the row's action menu to replace a saved encrypted value, regenerate a random value, or clear it.
- **Unique names**: plain and encrypted variables share one namespace, so a name can appear only once per deploy config.
- **Saved together**: every change in the table, including replacing, regenerating, and clearing encrypted variables, takes effect only when you save. The bottom action bar summarizes pending encrypted operations, and a failed save writes nothing.
- **Plain to encrypted**: turn on the switch; the current value is saved as the encrypted value.
- **Encrypted to plain**: the original value cannot be read back, so you must enter a new plain value. The secret is removed when you save.
- **Bulk import**: select **Paste .env** and enter one `KEY=VALUE` per line. `export` prefixes, full-line `#` comments, and quotes are supported. Imported variables are plain; you can choose whether to overwrite plain variables with the same name, and encrypted variables are never overwritten.

When the platform is upgraded to this version, any deploy config or shared configuration that has a plain variable and a secret with the same name keeps the secret and removes the plain variable. This matches the value that was already used at release time, so running instances are unaffected. The audit log records the removed names, never their values.

Build variables can exist at global, project-space, application, and deployment levels. The closest matching value wins. Build arguments and ordinary variables appear in build records and are not suitable for secrets.

## Shared configuration

Project shared configuration also rejects a plain variable and a secret with the same name. Its secrets are still managed separately in the shared configuration dialog and apply immediately. During release, shared configuration is merged first and the deploy config overrides matching names; a secret from any layer wins over a plain variable with the same name.

Use configuration files or secret files for mounted multiline content instead of placing it in a startup command.

After changing runtime configuration, save and redeploy when the page prompts. Saving desired configuration alone does not change an existing Pod.
