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

## Install the companion Skill for an AI agent

Copy the following prompt into an AI coding assistant that supports Skills. The companion Skill is published in [Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases), must exactly match the installed Luna CLI version, and is named `luna-devops-<version>.skill`.

```text
Install the Luna CLI companion `luna-devops` Skill in the current AI coding environment.

1. Run `luna version show output=json interactive=false agent=true` to read the installed Luna CLI version.
2. Open https://github.com/LiteyukiStudio/luna-cli/releases/latest. If the latest Release does not match the installed CLI version, select the Release whose version matches the CLI exactly.
3. Download the `luna-devops-<version>.skill` asset attached to that Release and install it using the Skill installation mechanism supported by the current AI environment.
4. Do not copy the Skill from the repository source tree, and do not mix different versions.
5. After installation, verify that the `luna-devops` Skill is available and report the CLI version, Skill version, and installation result. If the AI environment must be reloaded or the Skill only becomes available in the next conversation turn, state that explicitly.
```

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
