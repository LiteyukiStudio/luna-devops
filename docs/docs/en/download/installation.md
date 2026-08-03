# Install Luna CLI

The official Luna CLI npm package is `@liteyuki/luna-cli`. It requires Node.js `22.14.0` or later.

## Install

Install with npm:

```bash
npm install --global @liteyuki/luna-cli
```

You can also run `pnpm add --global @liteyuki/luna-cli`.

Verify the installation:

```bash
luna --version
```

Use a Node.js version manager or a user-level pnpm home instead of using `sudo` to work around global directory permissions.

## Sign in

Sign in to Luna DevOps:

```bash
luna login
luna whoami
```

For a self-hosted instance, specify its address with `luna login server=https://devops.example.com`. Then continue with [Use Luna CLI](./cli).

## Update

```bash
npm update --global @liteyuki/luna-cli
```

With pnpm, run `pnpm update --global @liteyuki/luna-cli`. See [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases) for prereleases and other release assets.

## Uninstall

```bash
npm uninstall --global @liteyuki/luna-cli
# or
pnpm remove --global @liteyuki/luna-cli
```

Run `luna logout` first to revoke the active login. Uninstalling the package does not remove local configuration under `~/.luna/`.
