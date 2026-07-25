import assert from "node:assert/strict";
import {
  mkdtempSync,
  readFileSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { packageSkills } from "../package-skills.mjs";
import { resolveSkillsReleaseMetadata } from "../release-metadata.mjs";

test("skills release metadata derives version from cli-skills tag", () => {
  assert.deepEqual(
    resolveSkillsReleaseMetadata("cli-skills-v1.2.3-beta.4"),
    {
      tag: "cli-skills-v1.2.3-beta.4",
      version: "1.2.3-beta.4",
      prerelease: "true",
    },
  );
});

test("skills release rejects another product tag", () => {
  assert.throws(
    () => resolveSkillsReleaseMetadata("cli-v1.2.3"),
    /must start with cli-skills-v/,
  );
});

test("skills package contains standard archives and compatibility metadata", () => {
  const root = mkdtempSync(join(tmpdir(), "luna-skills-release-"));
  try {
    const manifest = packageSkills({
      output: root,
      tag: "cli-skills-v1.2.3-beta.4",
      version: "1.2.3-beta.4",
      commit: "0123456789abcdef",
      requiresCli: ">=1.2.0 <2.0.0",
    });
    assert.ok(manifest.skills.length > 1);
    assert.equal(manifest.requires.lunaCli, ">=1.2.0 <2.0.0");
    assert.match(
      readFileSync(join(root, "RELEASE_NOTES.md"), "utf8"),
      /Required Luna CLI version/,
    );
    assert.match(
      readFileSync(join(root, "SHA256SUMS"), "utf8"),
      /\.skill/,
    );
    const firstChecksums = readFileSync(join(root, "SHA256SUMS"), "utf8");
    const rebuilt = packageSkills({
      output: root,
      tag: "cli-skills-v1.2.3-beta.4",
      version: "1.2.3-beta.4",
      commit: "0123456789abcdef",
      requiresCli: ">=1.2.0 <2.0.0",
    });
    assert.deepEqual(rebuilt, manifest);
    assert.equal(
      readFileSync(join(root, "SHA256SUMS"), "utf8"),
      firstChecksums,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
