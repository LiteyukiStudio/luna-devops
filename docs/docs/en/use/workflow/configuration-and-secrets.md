# Configuration and Secrets

A deployment can combine ordinary environment variables, ConfigMaps, configuration files, runtime Secrets, and secret files.

## Ordinary configuration

Enter environment variables as one `KEY=VALUE` per line. They are stored as ordinary configuration and may be readable by authorized clients, so **do not place passwords, tokens, or private keys here**.

Build variables can exist at global, project-space, application, and deployment levels. The closest matching value wins. Build arguments and ordinary variables appear in build records and are not suitable for secrets.

## Runtime Secrets

Save the deployment first, then add, replace, generate, or clear fields under **Runtime Secrets**:

- Values are stored encrypted and never displayed again.
- A blank edit keeps the current value. Clearing requires an explicit clear action.
- A runtime Secret overrides an ordinary variable with the same name during release.
- Luna DevOps can generate and store random credentials without returning plaintext to the browser.

Each Secret update succeeds or rolls back as a unit. Use configuration files or secret files for mounted multiline content instead of placing it in a startup command.

After changing runtime configuration, save and redeploy when the page prompts. Saving desired configuration alone does not change an existing Pod.
