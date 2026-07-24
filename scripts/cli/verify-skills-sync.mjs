import { mkdtempSync, readFileSync, readdirSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const skillsRoot = join(root, "ai-supports", "skills");
const lunaHome = mkdtempSync(join(tmpdir(), "luna-skills-sync-"));
const errors = [];

function listSkillFiles(directory) {
  return readdirSync(directory)
    .flatMap((entry) => {
      const path = join(directory, entry);
      return statSync(path).isDirectory() ? listSkillFiles(path) : [path];
    })
    .filter((path) => basename(path) === "SKILL.md");
}

function cliInvocation() {
  const tsx = join(root, "cli", "node_modules", ".bin", "tsx");
  try {
    if (statSync(tsx).isFile()) {
      return { command: tsx, prefix: [join(root, "cli", "src", "entry.ts")] };
    }
  } catch {
    // Release archives may only contain the built CLI.
  }

  return {
    command: process.execPath,
    prefix: [join(root, "cli", "dist", "entry.js")],
  };
}

function runCatalogPage(cursor) {
  const runner = cliInvocation();
  const args = [
    ...runner.prefix,
    "help",
    "catalog",
    "all=true",
    "limit=100",
    ...(cursor ? [`cursor=${cursor}`] : []),
    "agent=true",
  ];
  const result = spawnSync(runner.command, args, {
    cwd: root,
    encoding: "utf8",
    env: { ...process.env, LUNA_HOME: lunaHome },
  });

  if (result.status !== 0) {
    throw new Error(result.stderr || result.stdout || "unable to read Luna command catalog");
  }
  return JSON.parse(result.stdout);
}

function loadCommandCatalog() {
  const paths = new Set();
  let cursor;
  do {
    const envelope = runCatalogPage(cursor);
    for (const item of envelope.data.items) {
      paths.add(item.path);
    }
    cursor = envelope.data.nextCursor;
  } while (cursor);
  return paths;
}

function frontmatterName(source) {
  const frontmatter = source.match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1];
  return frontmatter?.match(/^name:\s*([^\r\n]+)$/m)?.[1]?.trim();
}

function validateSkill(path, commandPaths) {
  const source = readFileSync(path, "utf8");
  const skillDirectory = basename(dirname(path));
  const displayPath = relative(root, path);
  const name = frontmatterName(source);

  if (name !== skillDirectory) {
    errors.push(`${displayPath}: frontmatter name must equal directory name "${skillDirectory}"`);
  }
  if (skillDirectory !== "luna-devops-cli" && !source.includes("luna-devops-cli")) {
    errors.push(`${displayPath}: domain skills must reference luna-devops-cli`);
  }

  const commandPattern = /`luna\s+([a-z0-9-]+)\s+([a-z0-9-]+)([^`]*)`/g;
  for (const match of source.matchAll(commandPattern)) {
    const commandPath = `${match[1]}.${match[2]}`;
    if (!commandPaths.has(commandPath)) {
      errors.push(`${displayPath}: command "${commandPath}" is not present in machine Help`);
    }
    if (!match[3].includes("agent=true")) {
      errors.push(`${displayPath}: Agent command "${commandPath}" must include agent=true`);
    }
  }

  const staleClaims = [
    "CLI 可用前仅用于规划",
    "OAuth Device Code 会话可按 CLI 流程完成 Step-up MFA",
    "高风险操作必须创建服务端计划",
  ];
  for (const claim of staleClaims) {
    if (source.includes(claim)) {
      errors.push(`${displayPath}: contains stale capability claim "${claim}"`);
    }
  }
}

try {
  const commandPaths = loadCommandCatalog();
  const skillFiles = listSkillFiles(skillsRoot);
  for (const path of skillFiles) {
    validateSkill(path, commandPaths);
  }

  if (errors.length > 0) {
    console.error("Luna CLI Skills sync check failed:");
    for (const error of errors) {
      console.error(`- ${error}`);
    }
    process.exitCode = 1;
  } else {
    console.log(
      `Luna CLI Skills sync check passed: ${skillFiles.length} skills, ${commandPaths.size} commands.`,
    );
  }
} finally {
  rmSync(lunaHome, { recursive: true, force: true });
}
