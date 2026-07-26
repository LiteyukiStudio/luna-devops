import {
  fail,
  isMainModule,
  parseArguments,
  requiredArgument,
  writeGithubOutput,
} from "../cli/lib.mjs";
import { validateCliVersion } from "../cli/release-metadata.mjs";

const TAG_PREFIX = "cli-v";

export function resolveSkillsReleaseMetadata(tag) {
  if (!tag.startsWith(TAG_PREFIX)) {
    throw new Error(`Luna CLI Skills release tag must start with ${TAG_PREFIX}: ${tag}`);
  }

  const version = tag.slice(TAG_PREFIX.length);
  let match;
  try {
    match = validateCliVersion(version);
  } catch {
    throw new Error(`Luna CLI Skills release tag contains an invalid SemVer: ${tag}`);
  }

  return {
    tag,
    version,
    prerelease: String(Boolean(match[4])),
  };
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  writeGithubOutput(
    resolveSkillsReleaseMetadata(requiredArgument(args, "tag")),
  );
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
