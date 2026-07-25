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

export function listSkills() {
  return readdirSync(skillsRoot, { withFileTypes: true })
    .filter(entry => entry.isDirectory() && !ignoredEntries.has(entry.name))
    .map(entry => validateSkillDirectory(join(skillsRoot, entry.name)))
    .sort((left, right) => left.name.localeCompare(right.name));
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
  if (!requiresCli.trim()) {
    throw new Error("A required Luna CLI version range must be declared");
  }

  const outputDirectory = resolve(output);
  rmSync(outputDirectory, { recursive: true, force: true });
  mkdirSync(outputDirectory, { recursive: true });

  const skills = listSkills();
  const stagingDirectory = mkdtempSync(join(tmpdir(), "luna-cli-skills-"));
  try {
    for (const skill of skills) {
      copyDirectoryDeterministically(
        join(skillsRoot, skill.name),
        join(stagingDirectory, skill.name),
      );
    }

    const archives = [];
    for (const skill of skills) {
      const archiveName = `${skill.name}-${version}.skill`;
      const archivePath = join(outputDirectory, archiveName);
      runArchiveCommand(
        "zip",
        ["-X", "-q", "-r", archivePath, skill.name],
        stagingDirectory,
      );
      assertStandardSkillArchive(archivePath, skill.name);
      archives.push(archiveName);
    }

    const bundleName = `luna-cli-skills-${version}.zip`;
    runArchiveCommand(
      "zip",
      [
        "-X",
        "-q",
        "-r",
        join(outputDirectory, bundleName),
        ...skills.map(skill => skill.name),
      ],
      stagingDirectory,
    );
    runArchiveCommand(
      "unzip",
      ["-tqq", join(outputDirectory, bundleName)],
      repositoryRoot,
    );

    const packagedFiles = [...archives, bundleName].sort();
    writeChecksums(outputDirectory, packagedFiles);

    const manifest = {
      schemaVersion: 1,
      product: "Luna CLI Skills",
      tag,
      version,
      commit,
      requires: {
        lunaCli: requiresCli,
      },
      bundle: {
        archive: bundleName,
        sha256: sha256(join(outputDirectory, bundleName)),
      },
      skills: skills.map((skill) => ({
        name: skill.name,
        archive: `${skill.name}-${version}.skill`,
        sha256: sha256(join(outputDirectory, `${skill.name}-${version}.skill`)),
        files: skill.files,
      })),
    };
    writeFileSync(
      join(outputDirectory, "LUNA-CLI-SKILLS-MANIFEST.json"),
      `${JSON.stringify(manifest, null, 2)}\n`,
    );

    const notes = `# Luna CLI Skills ${version}

## 中文

- **必需 Luna CLI 版本：** \`${requiresCli}\`
- 单个 Skill 使用标准 \`.skill\` ZIP 格式，压缩包内只有一个同名 Skill 根目录。
- 可下载单个 \`.skill\` 文件，也可以下载 \`${bundleName}\` 一次安装整套 Skills。
- 使用前请通过 \`SHA256SUMS\` 校验下载文件。

## English

- **Required Luna CLI version:** \`${requiresCli}\`
- Each Skill uses the standard \`.skill\` ZIP format with one matching root Skill directory.
- Download an individual \`.skill\` archive or install the complete \`${bundleName}\` bundle.
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
