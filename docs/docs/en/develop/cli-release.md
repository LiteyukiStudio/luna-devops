# Release and artifact verification

Luna DevOps and Luna CLI are released from separate repositories:

| Product | Repository | Git tag | Release channels |
| --- | --- | --- | --- |
| Luna DevOps | `LiteyukiStudio/luna-devops` | `v1.2.3` | Container images and the platform GitHub Release |
| Luna CLI + Skill | `LiteyukiStudio/luna-cli` | `v1.2.3` | npm, binaries, one `.skill`, and the CLI GitHub Release |

Both repositories use standard `v*` tags, but their workflows and Releases are
independent. The CLI and Skill must share the same version, tag, commit, and
GitHub Release. The CLI can run without the Skill, while the Skill requires the
exact paired CLI version.

## Version source

The source `package.json.version` remains `0.0.0-development` and uses
`private: true` to prevent accidental publication from the checkout. A `v*` tag
in the CLI repository is the sole release-version source. The workflow writes
that version and removes `private` only in a temporary npm package, then injects
the same value into JavaScript artifacts and Bun binaries.

Prerelease suffixes select the npm dist-tag:

| Tag example | npm dist-tag | GitHub Release |
| --- | --- | --- |
| `v1.2.3` | `latest` | Release |
| `v1.2.3-rc.1` | `next` | Prerelease |
| `v1.2.3-beta.1` | `beta` | Prerelease |

## CI gates

Regular CLI CI:

1. installs locked dependencies and runs TypeScript, ESLint, tests, and builds;
2. validates the OpenAPI contract snapshot and paired Skill;
3. checks out Luna DevOps source read-only;
4. compares Gin routes, platform OpenAPI, the CLI machine catalog, and exact
   protocol classifications;
5. requires 100% coverage for ordinary business commands;
6. builds a real npm tarball and smoke-tests global npm and pnpm installation.

The CLI repository only reads platform source. It never pushes to the platform
repository and does not require shared history.

## Paired CLI and Skill release

A `v*` tag in the CLI repository triggers `cli-release.yml`. The same workflow
builds the CLI, validates the Skill structure and command references, and
creates deterministic artifacts in one GitHub Release:

- `luna-devops-<version>.skill`;
- `LUNA-CLI-SKILLS-MANIFEST.json`;
- the npm tarball and supported standalone binaries;
- `SHA256SUMS`, release manifest, SBOM, and provenance.

A `.skill` file is a ZIP archive with one `luna-devops` root directory. An
Agent reads the root `SKILL.md` for routing and loads only the required
`references/` files.

All new artifacts are available from
[Luna CLI Releases](https://github.com/LiteyukiStudio/luna-cli/releases).
Releases up to and including `0.0.12` remain in the former platform repository
and their historical links are preserved.

## npm Trusted Publishing

npm publishing uses GitHub OIDC Trusted Publishing instead of a long-lived
write token. Configure the package with:

- Organization or user: `LiteyukiStudio`
- Repository: `luna-cli`
- Workflow: `cli-release.yml`
- Environment: `npm`

The publishing job receives only the required `id-token: write` permission. If
the version already exists, the workflow compares npm integrity: identical
content is skipped, while different content fails and requires a new version.

## Platform contract source

The platform OpenAPI remains owned by `LiteyukiStudio/luna-devops`. Before a
CLI release, the coverage gate checks out a selected platform revision
read-only and records the revision and OpenAPI digest in artifact metadata.
This keeps releases independent while preserving a traceable compatibility
baseline.

## Artifact boundaries

- npm/pnpm with Node.js is the universal installation path.
- Linux glibc x64 and arm64 receive smoke-tested standalone binaries.
- macOS artifacts remain explicitly marked as test artifacts until Developer
  ID signing and notarization are available.
- Windows and Alpine/musl use the npm/pnpm and Node.js fallback.

After downloading a binary, verify `SHA256SUMS` and confirm in GitHub
Attestations that it came from the matching `LiteyukiStudio/luna-cli` tag and
release workflow.
