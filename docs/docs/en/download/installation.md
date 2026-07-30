# Install Luna CLI

The official Luna CLI npm package is `@liteyuki/luna-cli`. It requires Node.js `22.14.0` or later.

## Install

Choose either package manager:

```bash
npm install --global @liteyuki/luna-cli
```

```bash
pnpm add --global @liteyuki/luna-cli
```

Verify the installation:

```bash
luna --version
luna --help
```

Use a Node.js version manager or a user-level pnpm home instead of using `sudo` to work around global directory permissions.

## Sign in

Sign in to the official instance:

```bash
luna login
luna whoami
```

To use another Luna DevOps instance:

```bash
luna login server=https://devops.example.com
```

Then continue with [Use Luna CLI](./cli).

## Update

```bash
npm update --global @liteyuki/luna-cli
```

Or:

```bash
pnpm update --global @liteyuki/luna-cli
```

To try a prerelease, select the `beta` channel explicitly:

```bash
pnpm add --global @liteyuki/luna-cli@beta
```

Other release assets are available from [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases).

## Uninstall

```bash
npm uninstall --global @liteyuki/luna-cli
# or
pnpm remove --global @liteyuki/luna-cli
```

Run `luna logout` first to revoke the active login. Uninstalling the package does not remove local configuration under `~/.luna/`.
