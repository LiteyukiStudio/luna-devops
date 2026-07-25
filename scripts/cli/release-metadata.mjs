import {
  fail,
  isMainModule,
  parseArguments,
  requiredArgument,
  writeGithubOutput,
} from "./lib.mjs";

const SEMVER_PATTERN
  = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export function validateCliVersion(version) {
  const match = SEMVER_PATTERN.exec(version);
  if (!match) {
    throw new Error(`Invalid CLI SemVer: ${version}`);
  }
  const prereleaseIdentifiers = (match[4] ?? "").split(".").filter(Boolean);
  if (
    prereleaseIdentifiers.some(
      identifier => /^\d+$/.test(identifier) && identifier.length > 1 && identifier.startsWith("0"),
    )
  ) {
    throw new Error(`Invalid CLI SemVer: ${version}`);
  }
  return match;
}

export function resolveReleaseMetadata(tag) {
  if (!tag.startsWith("cli-v")) {
    throw new Error(`CLI release tag must start with cli-v: ${tag}`);
  }

  const version = tag.slice("cli-v".length);
  let match;
  try {
    match = validateCliVersion(version);
  } catch {
    throw new Error(`CLI release tag contains an invalid SemVer: ${tag}`);
  }

  const prerelease = match[4] ?? "";
  const npmTag = prerelease
    ? prerelease === "beta" || prerelease.startsWith("beta.")
      ? "beta"
      : "next"
    : "latest";

  return {
    tag,
    version,
    npm_tag: npmTag,
    prerelease: String(Boolean(prerelease)),
  };
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const tag = requiredArgument(args, "tag");
  writeGithubOutput(resolveReleaseMetadata(tag));
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
