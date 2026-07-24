# Install and Use

> Luna CLI has not completed its first public release. The package-manager commands below become available after a corresponding release. If npm returns 404, use the repository development workflow and do not download a similarly named package from an unofficial source.

## npm or pnpm

Stable channel:

```bash
npm install --global @liteyuki/luna-cli
pnpm add --global @liteyuki/luna-cli
```

Prereleases require an explicit channel:

```bash
npm install --global @liteyuki/luna-cli@next
pnpm add --global @liteyuki/luna-cli@beta
```

Verify the installation:

```bash
luna --version
luna help catalog output=json interactive=false
```

The npm distribution requires Node.js `22.14.0` or later. Use a Node.js version manager or a user-owned pnpm home instead of running a global install with `sudo`.

## Standalone binaries

The first stable release is planned to include Linux x64, Linux arm64, and Linux x64 musl binaries. After a release is available, download the matching asset and `SHA256SUMS`:

```bash
version="cli-vX.Y.Z"
asset="luna-linux-x64"
base="https://github.com/LiteyukiStudio/devops/releases/download/${version}"

curl -fL -o luna "${base}/${asset}"
curl -fL -o SHA256SUMS "${base}/SHA256SUMS"
grep " ${asset}$" SHA256SUMS | sed "s# ${asset}$# luna#" | sha256sum -c -
chmod +x luna
install -m 0755 luna "${HOME}/.local/bin/luna"
```

On macOS, use `shasum -a 256` instead of `sha256sum`. Until Apple Developer ID/notarization and Windows Authenticode are configured, desktop binaries are available only on prereleases, are explicitly suffixed with `-unsigned`, and are not recommended for production.

## Multi-instance contexts

Luna CLI is designed to manage more than one Luna DevOps instance. A context stores the instance URL, credential reference, default project, language, and output preference under `~/.luna/`.

Context commands use this form:

```bash
luna context set name=production server=https://devops.example.com
luna context list output=json interactive=false
luna context use name=production
luna context current
```

Automation should select its context or server explicitly and set:

```text
output=json interactive=false
```

Do not parse colored tables, column widths, or localized human output.

## Shell completion

The command registry supports Bash, Zsh, Fish, and PowerShell completion generation. Use `luna completion bash`, `luna completion zsh`, `luna completion fish`, or `luna completion powershell`. Inspect the machine-readable contract with `luna help command path=completion.zsh output=json interactive=false`.

## Uninstall

```bash
npm uninstall --global @liteyuki/luna-cli
pnpm remove --global @liteyuki/luna-cli
rm "${HOME}/.local/bin/luna"
```

Uninstalling does not silently delete `~/.luna/`. Revoke and remove credentials through the CLI logout flow. Delete the directory manually only after confirming that no contexts are needed.
