# Release Security

Luna CLI and the Luna DevOps platform use separate version and tag namespaces:

| Git tag | npm dist-tag | GitHub Release |
| --- | --- | --- |
| `cli-v1.2.3` | `latest` | Stable |
| `cli-v1.2.3-rc.1` | `next` | Prerelease |
| `cli-v1.2.3-beta.1` | `beta` | Prerelease |

Plain `v*` tags remain reserved for platform releases and do not trigger CLI publishing.

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
5. bump to a new unpublished version and use its `cli-v*` tag to verify OIDC publishing. Reusing the bootstrap version only exercises the idempotency check and does not call `npm publish`.

## CI gates

CLI changes run these checks:

1. Install the locked pnpm workspace.
2. Regenerate the API contract and reject drift.
3. Read machine Help and verify every paired Skill command, Agent argument, and capability boundary.
4. Run TypeScript typecheck, ESLint, unit tests, and the build.
5. Create a real npm tarball and validate its file allowlist.
6. Install the same tarball globally with npm and pnpm in clean temporary directories.
7. Build a Bun baseline binary for the Linux CI host and run command smoke tests.

The release workflow also builds an explicit target matrix:

- Linux x64 baseline;
- Linux arm64;
- Linux x64 musl baseline, with an additional Alpine smoke test;
- macOS arm64 and x64 test artifacts;
- Windows x64 test artifacts.

## Signing boundary

The repository does not currently have Apple Developer ID/notarization or Windows Authenticode credentials. The workflow does not claim that these artifacts are signed:

- stable releases contain only target-smoked Linux standalone binaries;
- prereleases may contain macOS and Windows test artifacts suffixed with `-unsigned`;
- unsigned desktop artifacts are not intended for production;
- desktop binaries enter the stable matrix only after platform signing and verification are integrated.

## npm Trusted Publishing

npm publishing uses GitHub OIDC Trusted Publishing without a long-lived write token. Maintainers configure the npm package with:

- Organization or user: `LiteyukiStudio`
- Repository: `devops`
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

Also inspect GitHub Release Attestations and confirm that the artifact was produced by `LiteyukiStudio/devops`, the `cli-release.yml` workflow, and the expected tag and commit.
