import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  requiredArgument,
  run,
} from "./lib.mjs";

export function smokeCommand(command, {
  expectedVersion,
  commandPrefix = [],
} = {}) {
  const lunaHome = mkdtempSync(join(tmpdir(), "luna-cli-smoke-"));
  const environment = {
    ...process.env,
    CI: "true",
    LANG: "C",
    LC_ALL: "C",
    LUNA_HOME: lunaHome,
    NO_COLOR: "1",
  };

  try {
    const version = run(command, [...commandPrefix, "--version"], {
      env: environment,
      timeout: 30_000,
    }).stdout.trim();
    if (!version || (expectedVersion && !version.includes(expectedVersion))) {
      throw new Error(
        `Unexpected version output: ${JSON.stringify(version)}${
          expectedVersion ? `; expected ${expectedVersion}` : ""
        }`,
      );
    }

    const localizedHelp = run(command, [...commandPrefix, "--help"], {
      env: {
        ...environment,
        LUNA_LANG: "zh-CN",
      },
      timeout: 30_000,
    }).stdout;
    for (const expectedText of ["用法：", "快速开始：", "输入规则："]) {
      if (!localizedHelp.includes(expectedText)) {
        throw new Error(
          `Localized help smoke did not contain ${JSON.stringify(expectedText)}`,
        );
      }
    }

    const help = run(
      command,
      [
        ...commandPrefix,
        "help",
        "catalog",
        "query=project",
        "limit=5",
        "output=json",
        "interactive=false",
      ],
      { env: environment, timeout: 30_000 },
    ).stdout.trim();

    let parsed;
    try {
      parsed = JSON.parse(help);
    } catch {
      throw new Error(`Help smoke did not return JSON: ${help.slice(0, 500)}`);
    }
    if (!parsed || typeof parsed !== "object") {
      throw new Error("Help smoke returned a non-object JSON value");
    }
  } finally {
    rmSync(lunaHome, { recursive: true, force: true });
  }
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  smokeCommand(requiredArgument(args, "command"), {
    expectedVersion: args.get("version"),
  });
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
