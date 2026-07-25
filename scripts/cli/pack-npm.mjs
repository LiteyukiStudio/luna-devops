import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  readJson,
  repositoryRoot,
  run,
  writeGithubOutput,
} from "./lib.mjs";
import { validateCliVersion } from "./release-metadata.mjs";

const ALLOWED_TOP_LEVEL = new Set([
  "LICENSE",
  "README.md",
  "bin",
  "dist",
  "package.json",
]);

export function validatePublishManifest(packageJson) {
  for (const name of ["preinstall", "install", "postinstall"]) {
    if (packageJson.scripts?.[name]) {
      throw new Error(`Published package must not define ${name}`);
    }
  }

  const workspaceDependencies = Object.entries(packageJson.dependencies ?? {})
    .filter(([, version]) => String(version).startsWith("workspace:"))
    .map(([name]) => name);
  if (workspaceDependencies.length > 0) {
    throw new Error(
      `Published runtime dependencies still use workspace protocol: ${workspaceDependencies.join(", ")}. `
      + "Bundle or publish those dependencies before releasing the CLI.",
    );
  }
}

export function validatePackFiles(files) {
  const rejected = [];
  for (const file of files) {
    const path = file.path.replace(/^package\//, "");
    const topLevel = path.split("/")[0];
    if (
      !ALLOWED_TOP_LEVEL.has(topLevel)
      || path.includes("/tests/")
      || path.endsWith(".map")
      || /(^|\/)\.env(?:\.|$)/.test(path)
    ) {
      rejected.push(path);
    }
  }

  if (rejected.length > 0) {
    throw new Error(`npm tarball contains disallowed files:\n${rejected.join("\n")}`);
  }
}

export function createPublishManifest(packageJson, version) {
  validateCliVersion(version);
  const { private: _private, ...publishablePackage } = packageJson;
  return {
    ...publishablePackage,
    version,
  };
}

export function packNpm(outputDirectory, version) {
  const cliDirectory = resolve(repositoryRoot, "cli");
  const packageJson = readJson(resolve(cliDirectory, "package.json"));
  const publishManifest = createPublishManifest(packageJson, version);
  validatePublishManifest(publishManifest);

  for (const required of ["bin/luna.js", "dist"]) {
    if (!existsSync(resolve(cliDirectory, required))) {
      throw new Error(`CLI build output is missing: cli/${required}`);
    }
  }

  const destination = resolve(repositoryRoot, outputDirectory);
  mkdirSync(destination, { recursive: true });
  const stagingDirectory = mkdtempSync(join(tmpdir(), "luna-cli-pack-"));
  let packed;
  try {
    for (const name of ["LICENSE", "README.md", "bin", "dist"]) {
      cpSync(resolve(cliDirectory, name), resolve(stagingDirectory, name), {
        recursive: true,
      });
    }
    writeFileSync(
      resolve(stagingDirectory, "package.json"),
      `${JSON.stringify(publishManifest, null, 2)}\n`,
    );
    packed = run(
      "npm",
      ["pack", "--json", `--pack-destination=${destination}`],
      { cwd: stagingDirectory, timeout: 120_000 },
    );
  } finally {
    rmSync(stagingDirectory, { recursive: true, force: true });
  }
  const result = JSON.parse(packed.stdout);
  if (!Array.isArray(result) || result.length !== 1) {
    throw new Error(`npm pack returned an unexpected result: ${packed.stdout}`);
  }
  if (result[0].version !== version) {
    throw new Error(
      `npm pack produced version ${result[0].version ?? "<missing>"}; expected ${version}`,
    );
  }

  validatePackFiles(result[0].files ?? []);
  const tarball = resolve(destination, result[0].filename);
  if (!existsSync(tarball)) {
    throw new Error(`npm pack did not create ${tarball}`);
  }

  return {
    tarball,
    filename: basename(tarball),
    version,
    integrity: result[0].integrity,
    packageSize: result[0].size,
    unpackedSize: result[0].unpackedSize,
  };
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const packageJson = readJson(resolve(repositoryRoot, "cli/package.json"));
  const result = packNpm(
    args.get("output") ?? "release/npm",
    args.get("version") ?? packageJson.version,
  );
  writeGithubOutput({
    tarball: result.tarball,
    filename: result.filename,
  });
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
