# Release Security

Luna DevOps, Luna CLI, and Luna CLI Skills use separate version and tag namespaces:

| Product | Git tag | Distribution |
| --- | --- | --- |
| Luna DevOps | `v1.2.3` | Container images and GitHub Release |
| Luna CLI | `cli-v1.2.3` | npm `latest`, binaries, and GitHub Release |
| Luna CLI | `cli-v1.2.3-rc.1` | npm `next` and GitHub Prerelease |
| Luna CLI | `cli-v1.2.3-beta.1` | npm `beta` and GitHub Prerelease |
| Luna CLI Skills | `cli-skills-v1.2.3` | `.skill`, bundle ZIP, and GitHub Release |
| Luna CLI Skills | `cli-skills-v1.2.3-beta.1` | `.skill`, bundle ZIP, and GitHub Prerelease |

Each prefix is consumed by a different workflow. Compatibility is maintained in
`release-compatibility.json`: Skills must declare a supported Luna CLI SemVer
range, while Luna CLI only recommends a Skills version and continues to work
without Skills.

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

## Luna CLI Skills releases

A `cli-skills-v*` tag triggers `cli-skills-release.yml`. The workflow reads the
tag version and `cliSkills.requiresCli` from `release-compatibility.json`,
validates Skill structure and CLI command synchronization, and publishes:

- one `<skill-name>-<version>.skill` archive per Skill;
- one complete `luna-cli-skills-<version>.zip` bundle;
- `LUNA-CLI-SKILLS-MANIFEST.json` with the required CLI range and artifact hashes;
- `SHA256SUMS` and GitHub OIDC build provenance.

A `.skill` is a ZIP archive with exactly one root directory matching the Skill
`name`, containing its `SKILL.md`, scripts, and references. Packaging fixes file
ordering and timestamps, so rebuilding the same tag produces the same hashes.
Download all artifacts from
[GitHub Releases](https://github.com/LiteyukiStudio/luna-devops/releases);
the documentation site does not mirror release binaries.

## Changelog synchronization

After any of the three release workflows succeeds, `changelog-sync.yml`
regenerates the Chinese and English Luna DevOps, Luna CLI, and Luna CLI Skills
changelogs from immutable tags. The synchronization job serially commits generated
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
