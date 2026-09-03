# Install Luna CLI

The official npm package is `@liteyuki/luna-cli` and requires Node.js `22.14.0` or later.

```bash
npm install --global @liteyuki/luna-cli
luna --version
```

You can also use `pnpm add --global @liteyuki/luna-cli`. Prefer a Node.js version manager or user-level pnpm home instead of using `sudo` to bypass global-directory permissions.

## Sign in

```bash
luna login
luna whoami
```

For a self-hosted instance, run `luna login server=https://devops.example.com`. Then continue to [Use Luna CLI](./index).

## Update and uninstall

```bash
npm update --global @liteyuki/luna-cli
npm uninstall --global @liteyuki/luna-cli
```

With pnpm, use `pnpm update --global` or `pnpm remove --global`. Run `luna logout` before uninstalling to revoke the login. Uninstall does not remove `~/.luna/` automatically.

The companion Agent Skill must match the CLI version. Download `luna-devops-<version>.skill` from the matching [Luna CLI Release](https://github.com/LiteyukiStudio/luna-cli/releases).

You can also send this prompt to an AI that supports Skills:

```plaintext
Use luna version show output=json interactive=false agent=true to check the installed Luna CLI version, then install the matching luna-devops-<version>.skill from https://github.com/LiteyukiStudio/luna-cli/releases. If luna is not detected, explain the issue and ask whether I want to install it; after approval, install the CLI using pnpm, npm, or a GitHub Release in that order, then continue.
```
