import { spawnSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  utimesSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, relative, resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  repositoryRoot,
  requiredArgument,
  sha256,
} from "../cli/lib.mjs";
import { validateCliVersion } from "../cli/release-metadata.mjs";

const skillsRoot = join(repositoryRoot, "ai-supports", "skills");
const skillName = "luna-devops";
const skillDirectory = join(skillsRoot, skillName);
const ignoredEntries = new Set([".DS_Store", "__pycache__"]);
const archiveTimestamp = new Date("2000-01-01T00:00:00.000Z");

function runArchiveCommand(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    stdio: "pipe",
  });
  if (result.error) {
    throw new Error(`${command} is required to package Luna CLI Skills`);
  }
  if (result.status !== 0) {
    throw new Error(
      `${command} ${args.join(" ")} failed: ${(result.stderr || result.stdout).trim()}`,
    );
  }
  return result.stdout;
}

function frontmatterName(source) {
  const frontmatter = source.match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1];
  return frontmatter?.match(/^name:\s*([^\r\n]+)$/m)?.[1]?.trim();
}

function frontmatterDescription(source) {
  const frontmatter = source.match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1];
  return frontmatter?.match(/^description:\s*([^\r\n]+)$/m)?.[1]?.trim();
}

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    if (ignoredEntries.has(entry.name)) return [];
    const path = join(directory, entry.name);
    if (lstatSync(path).isSymbolicLink()) {
      throw new Error(`Skill packages cannot contain symbolic links: ${path}`);
    }
    return entry.isDirectory() ? walk(path) : [path];
  });
}

function copyDirectoryDeterministically(source, destination) {
  mkdirSync(destination, { recursive: true });
  for (const entry of readdirSync(source, { withFileTypes: true })
    .filter(entry => !ignoredEntries.has(entry.name))
    .sort((left, right) => left.name.localeCompare(right.name))) {
    const sourcePath = join(source, entry.name);
    const destinationPath = join(destination, entry.name);
    if (lstatSync(sourcePath).isSymbolicLink()) {
      throw new Error(`Skill packages cannot contain symbolic links: ${sourcePath}`);
    }
    if (entry.isDirectory()) {
      copyDirectoryDeterministically(sourcePath, destinationPath);
      continue;
    }
    copyFileSync(sourcePath, destinationPath);
    chmodSync(destinationPath, statSync(sourcePath).mode);
    utimesSync(destinationPath, archiveTimestamp, archiveTimestamp);
  }
  utimesSync(destination, archiveTimestamp, archiveTimestamp);
}

export function validateSkillDirectory(directory) {
  const name = basename(directory);
  const skillFile = join(directory, "SKILL.md");
  let source;
  try {
    source = readFileSync(skillFile, "utf8");
  } catch {
    throw new Error(`${name} must contain SKILL.md`);
  }
  if (frontmatterName(source) !== name) {
    throw new Error(`${name}/SKILL.md frontmatter name must equal its directory name`);
  }
  if (!frontmatterDescription(source)) {
    throw new Error(`${name}/SKILL.md must declare a non-empty description`);
  }

  const files = walk(directory);
  if (files.length === 0) {
    throw new Error(`${name} does not contain packageable files`);
  }
  return {
    name,
    files: files.map(path => relative(skillsRoot, path)).sort(),
  };
}

export function loadSkill() {
  const unexpected = readdirSync(skillsRoot, { withFileTypes: true })
    .filter(entry => entry.isDirectory() && !ignoredEntries.has(entry.name))
    .filter(entry => entry.name !== skillName)
    .filter(entry => walk(join(skillsRoot, entry.name)).length > 0)
    .map(entry => entry.name);
  if (unexpected.length > 0) {
    throw new Error(
      `Luna CLI publishes one Skill; move these domains under ${skillName}/references: ${unexpected.join(", ")}`,
    );
  }
  return validateSkillDirectory(skillDirectory);
}

function archiveEntries(path) {
  return runArchiveCommand("unzip", ["-Z1", path], repositoryRoot)
    .split("\n")
    .filter(Boolean);
}

function assertStandardSkillArchive(path, skillName) {
  const entries = archiveEntries(path);
  const prefix = `${skillName}/`;
  if (!entries.includes(`${prefix}SKILL.md`)) {
    throw new Error(`${basename(path)} does not contain ${prefix}SKILL.md`);
  }
  if (entries.some(entry => !entry.startsWith(prefix))) {
    throw new Error(`${basename(path)} must contain exactly one root skill directory`);
  }
  runArchiveCommand("unzip", ["-tqq", path], repositoryRoot);
}

function writeChecksums(outputDirectory, files) {
  const contents = files
    .map(name => `${sha256(join(outputDirectory, name))}  ${name}`)
    .join("\n");
  writeFileSync(join(outputDirectory, "SHA256SUMS"), `${contents}\n`);
}

export function packageSkills({
  output,
  tag,
  version,
  commit,
  requiresCli,
}) {
  validateCliVersion(version);
  if (requiresCli !== version) {
    throw new Error(
      `Luna CLI Skills ${version} must require the exact CLI version ${version}`,
    );
  }

  const outputDirectory = resolve(output);
  rmSync(outputDirectory, { recursive: true, force: true });
  mkdirSync(outputDirectory, { recursive: true });

  const skill = loadSkill();
  const stagingDirectory = mkdtempSync(join(tmpdir(), "luna-cli-skills-"));
  try {
    copyDirectoryDeterministically(
      skillDirectory,
      join(stagingDirectory, skill.name),
    );

    const archiveName = `${skill.name}-${version}.skill`;
    const archivePath = join(outputDirectory, archiveName);
    runArchiveCommand(
      "zip",
      ["-X", "-q", "-r", archivePath, skill.name],
      stagingDirectory,
    );
    assertStandardSkillArchive(archivePath, skill.name);
    writeChecksums(outputDirectory, [archiveName]);

    const manifest = {
      schemaVersion: 2,
      product: "Luna CLI Skill",
      tag,
      version,
      commit,
      requires: {
        lunaCli: requiresCli,
      },
      skill: {
        name: skill.name,
        archive: archiveName,
        sha256: sha256(archivePath),
        files: skill.files,
        loading: "progressive-disclosure",
        references: skill.files.filter(path => path.includes("/references/")).length,
      },
    };
    writeFileSync(
      join(outputDirectory, "LUNA-CLI-SKILLS-MANIFEST.json"),
      `${JSON.stringify(manifest, null, 2)}\n`,
    );

    const notes = `# Luna CLI Skill ${version}

## 中文

- **必需 Luna CLI 版本：** \`${requiresCli}\`（必须与 Skill 版本完全一致）
- Release 只发布 \`${archiveName}\`，其中只有一个 \`luna-devops\` 根 Skill。
- 通用契约位于根 \`SKILL.md\`，业务领域位于 \`references/\` 并按任务需要加载。
- 使用前请通过 \`SHA256SUMS\` 校验下载文件。

## English

- **Required Luna CLI version:** \`${requiresCli}\` (must exactly match the Skill version)
- The release contains only \`${archiveName}\`, with one \`luna-devops\` root Skill.
- Shared contracts live in the root \`SKILL.md\`; domain references are loaded on demand.
- Verify downloaded files with \`SHA256SUMS\` before installation.
`;
    writeFileSync(join(outputDirectory, "RELEASE_NOTES.md"), notes);
    return manifest;
  } finally {
    rmSync(stagingDirectory, { recursive: true, force: true });
  }
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const manifest = packageSkills({
    output: requiredArgument(args, "output"),
    tag: requiredArgument(args, "tag"),
    version: requiredArgument(args, "version"),
    commit: requiredArgument(args, "commit"),
    requiresCli: requiredArgument(args, "requires-cli"),
  });
  process.stdout.write(`${JSON.stringify(manifest, null, 2)}\n`);
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
