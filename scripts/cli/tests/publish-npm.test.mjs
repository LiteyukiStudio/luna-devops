import assert from "node:assert/strict";
import test from "node:test";

import { resolvePublishIdentity } from "../publish-npm.mjs";

const sourcePackageJson = {
  name: "@liteyuki/luna-cli",
  version: "0.0.0-development",
  private: true,
};

test("uses the packed manifest as the npm release version source", () => {
  const identity = resolvePublishIdentity({
    sourcePackageJson,
    packedPackageJson: {
      name: "@liteyuki/luna-cli",
      version: "0.0.0-beta.4",
    },
    expectedVersion: "0.0.0-beta.4",
  });

  assert.deepEqual(identity, {
    name: "@liteyuki/luna-cli",
    version: "0.0.0-beta.4",
  });
});

test("rejects a tarball with an unexpected package identity or version", () => {
  assert.throws(
    () => resolvePublishIdentity({
      sourcePackageJson,
      packedPackageJson: {
        name: "@example/luna-cli",
        version: "0.0.0-beta.4",
      },
      expectedVersion: "0.0.0-beta.4",
    }),
    /Expected package/,
  );

  assert.throws(
    () => resolvePublishIdentity({
      sourcePackageJson,
      packedPackageJson: {
        name: "@liteyuki/luna-cli",
        version: "0.0.0-beta.3",
      },
      expectedVersion: "0.0.0-beta.4",
    }),
    /Expected version/,
  );
});

test("rejects a private tarball manifest", () => {
  assert.throws(
    () => resolvePublishIdentity({
      sourcePackageJson,
      packedPackageJson: {
        name: "@liteyuki/luna-cli",
        version: "0.0.0-beta.4",
        private: true,
      },
      expectedVersion: "0.0.0-beta.4",
    }),
    /must not be private/,
  );
});
