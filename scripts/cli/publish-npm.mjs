import { existsSync } from "node:fs";
import { resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  readJson,
  repositoryRoot,
  requiredArgument,
  run,
  sha512Integrity,
} from "./lib.mjs";
import { validateCliVersion } from "./release-metadata.mjs";

const REGISTRY = "https://registry.npmjs.org/";

export function resolvePublishIdentity({
  sourcePackageJson,
  packedPackageJson,
  expectedVersion,
}) {
  if (packedPackageJson.name !== sourcePackageJson.name) {
    throw new Error(
      `Expected package ${sourcePackageJson.name}, found ${packedPackageJson.name ?? "<missing>"}`,
    );
  }
  if (packedPackageJson.private === true) {
    throw new Error("Packed npm package must not be private");
  }

  const version = String(packedPackageJson.version ?? "");
  validateCliVersion(version);
  if (expectedVersion && version !== expectedVersion) {
    throw new Error(`Expected version ${expectedVersion}, found ${version}`);
  }

  return {
    name: packedPackageJson.name,
    version,
  };
}

export function readPackedManifest(path) {
  const result = run(
    "tar",
    ["-xOf", path, "package/package.json"],
    { timeout: 30_000 },
  );
  try {
    return JSON.parse(result.stdout);
  } catch {
    throw new Error("Unable to parse package/package.json from npm tarball");
  }
}

export function publishNpm({ tarball, npmTag, expectedVersion }) {
  const path = resolve(tarball);
  if (!existsSync(path)) {
    throw new Error(`npm tarball does not exist: ${path}`);
  }

  const sourcePackageJson = readJson(resolve(repositoryRoot, "cli/package.json"));
  const packedPackageJson = readPackedManifest(path);
  const identity = resolvePublishIdentity({
    sourcePackageJson,
    packedPackageJson,
    expectedVersion,
  });

  const packageVersion = `${identity.name}@${identity.version}`;
  const localIntegrity = sha512Integrity(path);
  const remote = run(
    "npm",
    ["view", packageVersion, "dist.integrity", "--json", `--registry=${REGISTRY}`],
    { allowFailure: true, timeout: 60_000 },
  );

  if (remote.status === 0) {
    const remoteIntegrity = JSON.parse(remote.stdout);
    if (remoteIntegrity !== localIntegrity) {
      throw new Error(
        `${packageVersion} is already published with different integrity. `
        + "Published npm versions are immutable; release a new version.",
      );
    }
    process.stdout.write(
      `${packageVersion} already exists with matching integrity; skipping publish.\n`,
    );
    return { published: false, integrity: localIntegrity };
  }

  const combinedError = `${remote.stdout ?? ""}\n${remote.stderr ?? ""}`;
  if (!/E404|404 Not Found|is not in this registry/i.test(combinedError)) {
    throw new Error(
      `Unable to determine whether ${packageVersion} exists:\n${combinedError.trim()}`,
    );
  }

  run(
    "npm",
    [
      "publish",
      path,
      "--access=public",
      `--tag=${npmTag}`,
      `--registry=${REGISTRY}`,
    ],
    { timeout: 180_000, stdio: "inherit" },
  );
  return { published: true, integrity: localIntegrity };
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const result = publishNpm({
    tarball: requiredArgument(args, "tarball"),
    npmTag: requiredArgument(args, "npm-tag"),
    expectedVersion: args.get("version"),
  });
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
