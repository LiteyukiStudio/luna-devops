import assert from "node:assert/strict";
import test from "node:test";

import {
  createPublishManifest,
  validatePackFiles,
  validatePublishManifest,
} from "../pack-npm.mjs";

test("injects the release version without changing the source manifest", () => {
  const source = {
    name: "@liteyuki/luna-cli",
    version: "0.0.0-development",
    private: true,
  };
  const published = createPublishManifest(source, "1.2.3-beta.1");

  assert.equal(source.version, "0.0.0-development");
  assert.equal(source.private, true);
  assert.equal(published.version, "1.2.3-beta.1");
  assert.equal("private" in published, false);
});

test("accepts the public npm package file whitelist", () => {
  assert.doesNotThrow(() => validatePackFiles([
    { path: "package/package.json" },
    { path: "package/README.md" },
    { path: "package/LICENSE" },
    { path: "package/bin/luna.js" },
    { path: "package/dist/entry.js" },
  ]));
});

test("rejects secrets, source maps and tests from the tarball", () => {
  assert.throws(
    () => validatePackFiles([
      { path: "package/dist/entry.js.map" },
      { path: "package/.env.production" },
      { path: "package/tests/auth.test.js" },
    ]),
    /disallowed files/,
  );
});

test("rejects lifecycle scripts and unresolved workspace dependencies", () => {
  assert.throws(
    () => validatePublishManifest({ scripts: { postinstall: "node setup.js" } }),
    /postinstall/,
  );
  assert.throws(
    () => validatePublishManifest({
      dependencies: { "@luna-devops/api-client": "workspace:*" },
    }),
    /workspace protocol/,
  );
});
