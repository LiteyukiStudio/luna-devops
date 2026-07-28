# Install and Use

> The official Luna CLI npm package is `@liteyuki/luna-cli`. Stable and
> prerelease channels use separate dist-tags. Install only from the official npm
> registry or the project GitHub Releases, not from similarly named third-party
> packages.

## npm or pnpm

Stable channel:

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

Prereleases require an explicit channel:

```bash
npm install --global @liteyuki/luna-cli@beta
pnpm add --global @liteyuki/luna-cli@beta
```

Verify the installation:

```bash
luna --version
luna --help
luna help catalog output=json interactive=false
```

The npm distribution requires Node.js `22.14.0` or later. Use a Node.js version manager or a user-owned pnpm home instead of running a global install with `sudo`.

npm/pnpm is the universal installation path for Windows, macOS, conventional Linux distributions, and Alpine/musl. It runs on the local Node.js runtime and does not depend on a Bun standalone executable.

## Standalone binaries

The first stable release includes only target-smoked Linux glibc x64 and arm64 binaries. After a release is available, download the matching asset and `SHA256SUMS`:

```bash
version="vX.Y.Z"
asset="luna-linux-x64"
base="https://github.com/LiteyukiStudio/luna-cli/releases/download/${version}"

curl -fL -o luna "${base}/${asset}"
curl -fL -o SHA256SUMS "${base}/SHA256SUMS"
grep " ${asset}$" SHA256SUMS | sed "s# ${asset}$# luna#" | sha256sum -c -
chmod +x luna
install -m 0755 luna "${HOME}/.local/bin/luna"
```

On macOS, use `shasum -a 256` instead of `sha256sum`. Until Apple Developer ID signing and notarization are configured, macOS binaries are available only on prereleases, are explicitly suffixed with `-unsigned`, and are not recommended for production.

Windows and Alpine/musl standalone binaries are not published yet. Install through npm or pnpm instead; this keeps Bun target-runtime downloads, Windows signing, and musl dynamic-library differences out of the supported release path.

## Server login

Luna CLI stores one active server/account credential in `~/.luna/auth.json`,
similar to `docker login`. Signing in to another server or account replaces the
active login and default project. There is no context-switching layer.

Without an explicit server, login targets the official instance:

```bash
luna login
```

This uses `https://devops.liteyuki.org`. Pass the server explicitly to log in
elsewhere:

```bash
luna login server=https://devops.example.com
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
```

Use `luna whoami` to inspect the active login and `luna logout` to revoke and
remove it. Automation should provide its credential explicitly and set:

```text
output=json interactive=false
```

Do not parse colored tables, column widths, or localized human output.

## Help and language

For human use, discover commands progressively:

```bash
luna
luna --help
luna project --help
luna project get-projects --help
```

Running `luna` without a subcommand displays localized root help and does not perform a remote operation. Command Help lists required parameters, types, input sources, risk, scopes, endpoint, and examples. Business parameters use `key=value`; use `key=@file` or `key=@-` for files, JSON, and multiline input.

Select Chinese for one command:

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
```

Precedence is `--lang` > `LUNA_LANG` > configured `language` > system locale >
English. Upgrade an older prerelease to the latest `beta` to receive the
complete locale detection behavior.

`latest` and `beta` are separate npm update channels. A regular
`pnpm update --global @liteyuki/luna-cli` does not switch a stable installation
to a prerelease. Install the beta explicitly when testing it:

```bash
pnpm add --global @liteyuki/luna-cli@beta
rehash
luna --version
which -a luna
pnpm list --global @liteyuki/luna-cli
```

If `luna --help` remains in English, verify the version and executable path
first. An older `luna` earlier in `PATH` may be shadowing the pnpm installation.

## Shell completion

The command registry supports Bash, Zsh, Fish, and PowerShell completion generation. Use `luna completion bash`, `luna completion zsh`, `luna completion fish`, or `luna completion powershell`. Inspect the machine-readable contract with `luna help command path=completion.zsh output=json interactive=false`.

## Uninstall

```bash
npm uninstall --global @liteyuki/luna-cli
pnpm remove --global @liteyuki/luna-cli
rm "${HOME}/.local/bin/luna"
```

Uninstalling does not silently delete `~/.luna/`. Revoke and remove credentials
with `luna logout`. Delete the directory manually only after confirming that no
local login data is needed.
