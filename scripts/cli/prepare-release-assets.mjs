import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  rmSync,
  statSync,
} from "node:fs";
import { basename, join, resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  requiredArgument,
} from "./lib.mjs";

const STABLE_BINARIES = new Set([
  "luna-linux-arm64",
  "luna-linux-x64",
]);
const PRERELEASE_BINARIES = new Set([
  ...STABLE_BINARIES,
  "luna-darwin-arm64-unsigned",
  "luna-darwin-x64-unsigned",
]);
const SKILLS_MANIFEST = "LUNA-CLI-SKILLS-MANIFEST.json";

function isSkillsAsset(name) {
  return /^luna-devops-.+\.skill$/.test(name)
    || name === SKILLS_MANIFEST;
}

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

export function prepareReleaseAssets({ input, output, prerelease }) {
  const inputDirectory = resolve(input);
  const outputDirectory = resolve(output);
  if (!existsSync(inputDirectory)) {
    throw new Error(`Artifact input directory does not exist: ${inputDirectory}`);
  }

  rmSync(outputDirectory, { recursive: true, force: true });
  mkdirSync(outputDirectory, { recursive: true });

  const allowedBinaries = prerelease
    ? PRERELEASE_BINARIES
    : STABLE_BINARIES;
  const files = walk(inputDirectory);
  const selected = files.filter((path) => {
    const name = basename(path);
    return allowedBinaries.has(name)
      || name.endsWith(".tgz")
      || isSkillsAsset(name);
  });

  const names = new Set();
  for (const path of selected) {
    const name = basename(path);
    if (names.has(name)) {
      throw new Error(`Duplicate release artifact: ${name}`);
    }
    if (!statSync(path).size) {
      throw new Error(`Release artifact is empty: ${path}`);
    }
    names.add(name);
    copyFileSync(path, join(outputDirectory, name));
  }

  for (const required of [...STABLE_BINARIES]) {
    if (!names.has(required)) {
      throw new Error(`Required Linux release artifact is missing: ${required}`);
    }
  }
  if (![...names].some(name => name.endsWith(".tgz"))) {
    throw new Error("npm tarball is missing from release artifacts");
  }
  const skillArchives = [...names].filter(name => /^luna-devops-.+\.skill$/.test(name));
  if (skillArchives.length !== 1) {
    throw new Error("exactly one paired luna-devops Skill archive is required");
  }
  if (!names.has(SKILLS_MANIFEST)) {
    throw new Error("paired Luna DevOps Skill manifest is missing from release artifacts");
  }
  if (prerelease) {
    for (const required of PRERELEASE_BINARIES) {
      if (!names.has(required)) {
        throw new Error(`Prerelease artifact is missing: ${required}`);
      }
    }
  }

  return [...names].sort();
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const files = prepareReleaseAssets({
    input: requiredArgument(args, "input"),
    output: requiredArgument(args, "output"),
    prerelease: args.get("prerelease") === "true",
  });
  process.stdout.write(`${JSON.stringify({ files }, null, 2)}\n`);
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
