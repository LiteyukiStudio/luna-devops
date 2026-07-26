import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { prepareReleaseAssets } from "../prepare-release-assets.mjs";
import { generateReleaseManifest } from "../release-manifest.mjs";

function hash(path) {
  return createHash("sha256")
    .update(readFileSync(path))
    .digest("hex");
}

function fixture({
  version = "1.2.3",
  tag = `cli-v${version}`,
  commit = "0123456789abcdef",
} = {}) {
  const root = mkdtempSync(join(tmpdir(), "luna-release-test-"));
  const input = join(root, "input");
  const output = join(root, "output");
  mkdirSync(input);
  for (const name of [
    "luna-linux-arm64",
    "luna-linux-x64",
    "luna-darwin-arm64-unsigned",
    "luna-darwin-x64-unsigned",
    `liteyukistudio-luna-cli-${version}.tgz`,
    `luna-cli-core-${version}.skill`,
    `luna-cli-skills-${version}.zip`,
  ]) {
    writeFileSync(join(input, name), name);
  }
  const skillArchive = `luna-cli-core-${version}.skill`;
  const bundleArchive = `luna-cli-skills-${version}.zip`;
  writeFileSync(
    join(input, "LUNA-CLI-SKILLS-MANIFEST.json"),
    `${JSON.stringify({
      schemaVersion: 1,
      product: "Luna CLI Skills",
      tag,
      version,
      commit,
      requires: {
        lunaCli: version,
      },
      bundle: {
        archive: bundleArchive,
        sha256: hash(join(input, bundleArchive)),
      },
      skills: [
        {
          name: "luna-cli-core",
          archive: skillArchive,
          sha256: hash(join(input, skillArchive)),
          files: ["luna-cli-core/SKILL.md"],
        },
      ],
    }, null, 2)}\n`,
  );
  return { root, input, output, version, tag, commit };
}

test("stable releases omit unsigned desktop binaries", () => {
  const { root, input, output } = fixture();
  try {
    const files = prepareReleaseAssets({
      input,
      output,
      prerelease: false,
    });
    assert.deepEqual(files, [
      "LUNA-CLI-SKILLS-MANIFEST.json",
      "liteyukistudio-luna-cli-1.2.3.tgz",
      "luna-cli-core-1.2.3.skill",
      "luna-cli-skills-1.2.3.zip",
      "luna-linux-arm64",
      "luna-linux-x64",
    ]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("manifest records hashes and unsigned prerelease limitations", () => {
  const version = "1.2.3-beta.1";
  const tag = `cli-v${version}`;
  const commit = "0123456789abcdef";
  const { root, input, output } = fixture({ version, tag, commit });
  try {
    prepareReleaseAssets({ input, output, prerelease: true });
    const manifest = generateReleaseManifest({
      directory: output,
      tag,
      version,
      commit,
      prerelease: true,
      npmTag: "beta",
    });
    assert.equal(manifest.files.length, 8);
    assert.equal(
      manifest.verification.unsignedDesktopArtifacts.length,
      2,
    );
    assert.equal(
      manifest.paired.lunaCliSkills.version,
      version,
    );
    assert.equal(manifest.paired.lunaCliSkills.requiredCli, version);
    assert.equal(manifest.paired.lunaCliSkills.skills, 1);
    assert.match(
      readFileSync(join(output, "SHA256SUMS"), "utf8"),
      /luna-linux-x64/,
    );
    assert.match(
      readFileSync(join(output, "RELEASE_NOTES.md"), "utf8"),
      /@liteyuki\/luna-cli@beta/,
    );
    assert.ok(
      readFileSync(join(output, "RELEASE_NOTES.md"), "utf8")
        .includes(`Luna CLI Skills：\`${version}\``),
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("release assembly rejects missing paired Skills assets", () => {
  const { root, input, output } = fixture();
  try {
    rmSync(join(input, "luna-cli-core-1.2.3.skill"));
    assert.throws(
      () => prepareReleaseAssets({ input, output, prerelease: false }),
      /Skills archives are missing/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("release manifest rejects a mismatched paired Skills version", () => {
  const { root, input, output, tag, commit } = fixture();
  try {
    prepareReleaseAssets({ input, output, prerelease: false });
    const manifestPath = join(output, "LUNA-CLI-SKILLS-MANIFEST.json");
    const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
    manifest.version = "1.2.4";
    writeFileSync(manifestPath, `${JSON.stringify(manifest)}\n`);
    assert.throws(
      () => generateReleaseManifest({
        directory: output,
        tag,
        version: "1.2.3",
        commit,
        prerelease: false,
        npmTag: "latest",
      }),
      /does not match CLI version/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("release manifest rejects tampered paired Skills archives", () => {
  const { root, input, output, version, tag, commit } = fixture();
  try {
    prepareReleaseAssets({ input, output, prerelease: false });
    writeFileSync(
      join(output, `luna-cli-core-${version}.skill`),
      "tampered",
    );
    assert.throws(
      () => generateReleaseManifest({
        directory: output,
        tag,
        version,
        commit,
        prerelease: false,
        npmTag: "latest",
      }),
      /checksum does not match/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
