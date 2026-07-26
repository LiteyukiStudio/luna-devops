# Release Security

Luna DevOps has its own release version. Luna CLI and the Luna DevOps Skill share the
same version, tag, commit, and GitHub Release:

| Product | Git tag | Distribution |
| --- | --- | --- |
| Luna DevOps | `v1.2.3` | Container images and GitHub Release |
| Luna CLI + Skill | `cli-v1.2.3` | npm `latest`, binaries, one `.skill` archive, and GitHub Release |
| Luna CLI + Skill | `cli-v1.2.3-rc.1` | npm `next`, CLI/Skill artifacts, and GitHub Prerelease |
| Luna CLI + Skill | `cli-v1.2.3-beta.1` | npm `beta`, CLI/Skill artifacts, and GitHub Prerelease |

The `v*` and `cli-v*` prefixes are consumed by different workflows.
`release-compatibility.json` declares the release policy: the Skill must use the
exact same version as the CLI, and a release fails when its artifact is missing
or mismatched. The CLI itself still works without installing the Skill.

The source `cli/package.json.version` stays at `0.0.0-development` and only identifies a development checkout. The source manifest also uses `private: true` to prevent accidental publication from the working tree. A `cli-v*` tag is the sole release-version source: the workflow validates its SemVer, removes the private marker and writes that version into a temporary npm package manifest, then injects the same value into the npm JavaScript build and every Bun binary. A release therefore does not require a package manifest version commit. The publishing stage reads `package/package.json` directly from the tested tarball to validate its name, version, and private state instead of comparing the tag with the source placeholder version.

## Bootstrapping the npm package

npm does not require an empty package to be created in advance. The first publish command creates the public scoped package `@liteyuki/luna-cli`:

```bash
npm publish <verified-tarball.tgz> --access public --tag next
```

Before the bootstrap publish:

1. confirm that the `@liteyuki` npm organization exists and the maintainer may create public packages;
2. enable 2FA and build, pack, and smoke-test the tarball from a clean environment;
3. use a prerelease version with the `next` tag instead of claiming the first stable version;
4. configure the GitHub Actions Trusted Publisher in the package settings after the package exists;
5. create a `cli-v*` tag for a new unpublished version to verify OIDC publishing. Reusing the bootstrap tag version only exercises the idempotency check and does not call `npm publish`.

## CI gates

CLI changes run these checks:

1. Install the locked pnpm workspace.
2. Regenerate the API contract and reject drift.
3. Read machine Help and verify every paired Skill command, Agent argument, and capability boundary.
4. Run TypeScript typecheck, ESLint, unit tests, and the build.
5. Create a real npm tarball and validate its file allowlist.
6. Install the same tarball globally with npm and pnpm in clean temporary directories.
7. Build a Bun baseline binary for the Linux CI host and run command smoke tests.

The release gate additionally asserts that the tarball manifest, the npm-installed `luna --version`, and every standalone binary report the tag version.

## Paired CLI and Skill releases

A `cli-v*` tag triggers `cli-release.yml`. The same workflow builds the CLI,
validates Skill structure and command synchronization, and publishes into one
GitHub Release:

- one `luna-devops-<version>.skill` archive containing the root `SKILL.md` and domain-specific `references/`;
- `LUNA-CLI-SKILLS-MANIFEST.json` with the exact matching CLI version, progressive-loading mode, and artifact hash;
- the npm package, standalone binaries, `SHA256SUMS`, and GitHub OIDC build provenance.

The release gate requires the Skill and CLI to have the same version, tag, and
commit, and `requires.lunaCli` must be that exact version. A missing individual
Skill or manifest aborts the release. `cli-skills-release.yml` remains
only as a manual packaging validation workflow and no longer creates a separate
Release.

A `.skill` is a ZIP archive with exactly one `luna-devops` root directory. An
agent reads the root `SKILL.md` for domain routing, then loads only the relevant
documents under `references/` for the current task instead of injecting every
domain into context. Packaging fixes file ordering and timestamps, so rebuilding
the same tag produces the same hashes.
Download all artifacts from
[GitHub Releases](https://github.com/LiteyukiStudio/luna-devops/releases);
the documentation site does not mirror release binaries.

## Changelog synchronization

After the platform or CLI release workflow succeeds, `changelog-sync.yml`
regenerates three Chinese and English changelog views for Luna DevOps, Luna CLI,
and the Luna DevOps Skill from immutable tags. The synchronization job serially commits generated
content to `main`, rebases and retries when concurrent updates occur, then
explicitly dispatches `Build & Publish Containers`. This explicit dispatch makes
sure a commit created with `GITHUB_TOKEN` still rebuilds the documentation site.
When there is no content change, the job exits without creating a workflow loop.

The release workflow also builds an explicit target matrix:

- Linux x64 baseline;
- Linux arm64;
- macOS arm64 and x64 prerelease test artifacts.

Windows and Alpine/musl are intentionally outside the standalone-binary matrix. They use the npm or pnpm distribution on Node.js `22.14.0` or later. The release gate therefore promises only artifacts that execute on their target runner, without relying on a build-time Bun target-runtime download or extra musl dynamic libraries on the user's machine.

## Signing boundary

The repository does not currently have Apple Developer ID signing and notarization credentials. The workflow does not claim that macOS artifacts are signed:

- stable releases contain only target-smoked Linux standalone binaries;
- prereleases may contain macOS test artifacts suffixed with `-unsigned`;
- unsigned macOS artifacts are not intended for production;
- desktop binaries enter the stable matrix only after platform signing and verification are integrated.

## npm Trusted Publishing

npm publishing uses GitHub OIDC Trusted Publishing without a long-lived write token. Maintainers configure the npm package with:

- Organization or user: `LiteyukiStudio`
- Repository: `luna-devops`
- Workflow: `cli-release.yml`
- Environment: `npm`

The GitHub `npm` Environment should require protected tags and maintainer approval. The publishing job grants `id-token: write` and does not set `NPM_TOKEN` or `NODE_AUTH_TOKEN`.

When a version already exists, the workflow compares npm `dist.integrity`:

- matching content: skip npm publishing and continue repairing the GitHub Release;
- different content: fail immediately and require a new version.

## Verify downloads

Each GitHub Release contains:

- `SHA256SUMS`;
- `RELEASE-MANIFEST.json`;
- the same-version `luna-devops-<version>.skill` and `LUNA-CLI-SKILLS-MANIFEST.json`;
- an SPDX JSON SBOM;
- GitHub OIDC build provenance;
- an SBOM attestation bundle.

At minimum, verify SHA-256:

```bash
grep " luna-linux-x64$" SHA256SUMS | sha256sum -c -
```

Also inspect GitHub Release Attestations and confirm that the artifact was
produced by `LiteyukiStudio/luna-devops`, the expected release workflow, tag,
and commit.
