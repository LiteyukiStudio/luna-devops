import assert from "node:assert/strict";
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

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "luna-release-test-"));
  const input = join(root, "input");
  const output = join(root, "output");
  mkdirSync(input);
  for (const name of [
    "luna-linux-arm64",
    "luna-linux-x64",
    "luna-darwin-arm64-unsigned",
    "luna-darwin-x64-unsigned",
    "liteyukistudio-luna-cli-1.2.3.tgz",
  ]) {
    writeFileSync(join(input, name), name);
  }
  return { root, input, output };
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
      "liteyukistudio-luna-cli-1.2.3.tgz",
      "luna-linux-arm64",
      "luna-linux-x64",
    ]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("manifest records hashes and unsigned prerelease limitations", () => {
  const { root, input, output } = fixture();
  try {
    prepareReleaseAssets({ input, output, prerelease: true });
    const manifest = generateReleaseManifest({
      directory: output,
      tag: "cli-v1.2.3-beta.1",
      version: "1.2.3-beta.1",
      commit: "0123456789abcdef",
      prerelease: true,
      npmTag: "beta",
    });
    assert.equal(manifest.files.length, 5);
    assert.equal(
      manifest.verification.unsignedDesktopArtifacts.length,
      2,
    );
    assert.match(
      readFileSync(join(output, "SHA256SUMS"), "utf8"),
      /luna-linux-x64/,
    );
    assert.match(
      readFileSync(join(output, "RELEASE_NOTES.md"), "utf8"),
      /@liteyuki\/luna-cli@beta/,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
