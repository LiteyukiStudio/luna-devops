import { chmodSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  readJson,
  repositoryRoot,
  requiredArgument,
  run,
} from "./lib.mjs";
import { validateCliVersion } from "./release-metadata.mjs";

export function buildBinary({ target, output, version }) {
  validateCliVersion(version);
  const outputPath = resolve(repositoryRoot, output);
  mkdirSync(dirname(outputPath), { recursive: true });

  run(
    "bun",
    [
      "build",
      "cli/src/entry.ts",
      "--compile",
      `--target=${target}`,
      `--outfile=${outputPath}`,
      "--define",
      `__LUNA_CLI_VERSION__:${JSON.stringify(version)}`,
    ],
    {
      stdio: "inherit",
      timeout: 300_000,
    },
  );

  if (process.platform !== "win32") {
    chmodSync(outputPath, 0o755);
  }
  return outputPath;
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const packageJson = readJson(resolve(repositoryRoot, "cli/package.json"));
  buildBinary({
    target: requiredArgument(args, "target"),
    output: requiredArgument(args, "output"),
    version: args.get("version") ?? packageJson.version,
  });
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
