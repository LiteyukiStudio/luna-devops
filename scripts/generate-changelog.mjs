import {
  mkdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const repositoryUrl = "https://github.com/LiteyukiStudio/luna-devops";
const compatibilityFile = "release-compatibility.json";

const tracks = {
  devops: {
    title: "Luna DevOps",
    tagPattern: "v[0-9]*",
    prefix: "v",
    output: "index.md",
    paths: [
      ".",
      ":(exclude)cli/**",
      ":(exclude)ai-supports/skills/**",
      ":(exclude)scripts/cli/**",
      ":(exclude)scripts/skills/**",
    ],
  },
  cli: {
    title: "Luna CLI",
    tagPattern: "cli-v*",
    prefix: "cli-v",
    output: "cli.md",
    paths: [
      "cli",
      "packages",
      "scripts/cli",
      "ai-supports",
      "scripts/skills",
      "notes/cli-spec.md",
      compatibilityFile,
    ],
  },
  "cli-skills": {
    title: "Luna DevOps Skill",
    tagPatterns: ["cli-v*", "cli-skills-v*"],
    prefixes: ["cli-v", "cli-skills-v"],
    output: "cli-skills.md",
    paths: [
      "ai-supports",
      "scripts/skills",
      compatibilityFile,
    ],
  },
};

const categoryOrder = [
  "added",
  "fixed",
  "performance",
  "docs",
  "changed",
  "other",
];

const categoryLabels = {
  zh: {
    added: "新增",
    fixed: "修复",
    performance: "性能",
    docs: "文档",
    changed: "调整",
    other: "其他",
  },
  en: {
    added: "Added",
    fixed: "Fixed",
    performance: "Performance",
    docs: "Docs",
    changed: "Changed",
    other: "Other",
  },
};

function git(args, options = {}) {
  try {
    return execFileSync("git", args, {
      cwd: root,
      encoding: "utf8",
      stdio: ["ignore", "pipe", options.allowFailure ? "ignore" : "pipe"],
    }).trim();
  } catch (error) {
    if (options.allowFailure) {
      return "";
    }
    throw error;
  }
}

function categoryForSubject(subject) {
  const type = /^([A-Za-z]+)(?:\([^)]*\))?(?:!)?(?:\s[^:]*)?:/
    .exec(subject)?.[1]
    ?.toLowerCase();
  if (type === "feat") return "added";
  if (type === "fix" || type === "revert") return "fixed";
  if (type === "perf") return "performance";
  if (type === "docs") return "docs";
  if (["refactor", "style", "test", "build", "ci", "chore"].includes(type)) {
    return "changed";
  }
  return "other";
}

function escapeMarkdown(value) {
  return value.replaceAll("\\", "\\\\").replaceAll("[", "\\[").replaceAll("]", "\\]");
}

function trackTags(trackName, track) {
  const patterns = track.tagPatterns ?? [track.tagPattern];
  const output = git([
    "tag",
    "--list",
    ...patterns,
    "--sort=-v:refname",
  ]);
  const tags = output ? output.split("\n").filter(Boolean) : [];
  if (trackName !== "cli-skills") return tags;
  return tags.filter((tag) => {
    if (tag.startsWith("cli-skills-v")) return true;
    return isPairedSkillsRelease(compatibilityAtTag(tag));
  });
}

function isPairedSkillsRelease(metadata) {
  return ["bundled", "single-progressive-skill"].includes(
    metadata?.cliSkills?.release,
  );
}

function commitsForRange(track, range) {
  const output = git([
    "log",
    "--no-merges",
    "--format=%H%x1f%s",
    range,
    "--",
    ...track.paths,
  ]);
  if (!output) return [];
  return output.split("\n").map((line) => {
    const [hash, subject] = line.split("\x1f");
    return { hash, subject, category: categoryForSubject(subject) };
  });
}

function compatibilityAtTag(tag) {
  const source = git(["show", `${tag}:${compatibilityFile}`], {
    allowFailure: true,
  });
  if (!source) return null;
  try {
    return JSON.parse(source);
  } catch {
    return null;
  }
}

function compatibilityLines(trackName, metadata, version, lang) {
  if (metadata?.cliSkills?.release === "single-progressive-skill") {
    return lang === "zh"
      ? [`**配套关系：** Luna CLI 与 Luna DevOps Skill 均为 \`${version}\`，必须精确同版本使用。`]
      : [`**Pairing:** Luna CLI and the Luna DevOps Skill are both \`${version}\` and must match exactly.`];
  }
  if (metadata?.cliSkills?.release === "bundled") {
    return lang === "zh"
      ? [`**配套关系：** Luna CLI 与 Luna CLI Skills 均为 \`${version}\`，必须精确同版本使用。`]
      : [`**Pairing:** Luna CLI and Luna CLI Skills are both \`${version}\` and must match exactly.`];
  }
  if (trackName === "cli") {
    const version = metadata?.cli?.recommendedSkills;
    return lang === "zh"
      ? [`**建议配套 Luna CLI Skills：** ${version ? `\`${version}\`` : "此版本未声明"}`]
      : [`**Recommended Luna CLI Skills:** ${version ? `\`${version}\`` : "Not declared for this release"}`];
  }
  if (trackName === "cli-skills") {
    const range = metadata?.cliSkills?.requiresCli;
    return lang === "zh"
      ? [`**必需 Luna CLI 版本：** ${range ? `\`${range}\`` : "此版本未声明"}`]
      : [`**Required Luna CLI version:** ${range ? `\`${range}\`` : "Not declared for this release"}`];
  }
  return [];
}

function versionForTag(track, tag) {
  const prefixes = track.prefixes ?? [track.prefix];
  const prefix = prefixes.find(candidate => tag.startsWith(candidate));
  if (!prefix) {
    throw new Error(`Tag ${tag} does not match the configured prefixes`);
  }
  return tag.slice(prefix.length);
}

function releaseSection(trackName, track, tag, previousTag, lang) {
  const version = versionForTag(track, tag);
  const range = previousTag ? `${previousTag}..${tag}` : tag;
  const date = git(["log", "-1", "--format=%cs", tag]);
  const releaseUrl = `${repositoryUrl}/releases/tag/${encodeURIComponent(tag)}`;
  const sourceUrl = `${repositoryUrl}/tree/${encodeURIComponent(tag)}`;
  const lines = [`## ${version}`, ""];

  if (lang === "zh") {
    lines.push(`发布日期：${date}`, "");
    lines.push(`[GitHub Release](${releaseUrl}) · [查看版本代码](${sourceUrl})`, "");
  } else {
    lines.push(`Release date: ${date}`, "");
    lines.push(`[GitHub Release](${releaseUrl}) · [View tag source](${sourceUrl})`, "");
  }

  const compatibility = compatibilityLines(
    trackName,
    compatibilityAtTag(tag),
    version,
    lang,
  );
  if (compatibility.length > 0) {
    lines.push(...compatibility, "");
  }

  const commits = commitsForRange(track, range);
  for (const category of categoryOrder) {
    const matches = commits.filter(commit => commit.category === category);
    if (matches.length === 0) continue;
    lines.push(`### ${categoryLabels[lang][category]}`, "");
    for (const commit of matches) {
      const shortHash = commit.hash.slice(0, 7);
      lines.push(
        `- ${escapeMarkdown(commit.subject)} ([${shortHash}](${repositoryUrl}/commit/${commit.hash}))`,
      );
    }
    lines.push("");
  }

  if (commits.length === 0) {
    lines.push(
      lang === "zh"
        ? "此版本没有匹配当前产品轨道的提交记录。"
        : "No commits matched this product release track.",
      "",
    );
  }
  return lines.join("\n");
}

function pageIntroduction(trackName, lang) {
  if (lang === "zh") {
    if (trackName === "cli") {
      return [
        "这里记录 Luna CLI 的公开版本变化。CLI 可以独立使用；新版本会在同一个 Release 中强制附带完全同版本的 Luna DevOps Skill。",
        "",
        "当前开发线采用 CLI 与 Skill 同版本、同 tag、同 Release 的绑定策略。",
        "",
        `安装与下载请前往 [GitHub Releases](${repositoryUrl}/releases)。`,
      ];
    }
    if (trackName === "cli-skills") {
      return [
        "这里记录 Luna DevOps Skill 的公开版本变化。新版本不再独立发版，而是随同版本 Luna CLI 一起发布。",
        "",
        "当前只发布一个 `luna-devops` Skill；根 `SKILL.md` 负责领域路由，具体说明从 `references/` 按需加载。Skill 强依赖完全相同版本的 Luna CLI，历史独立 Skills Release 仍保留用于追溯。",
        "",
        `标准 \`luna-devops-<version>.skill\` 请前往 [GitHub Releases](${repositoryUrl}/releases) 下载。`,
      ];
    }
    return [
      "这里记录 Luna DevOps 平台本体的公开版本变化。",
      "",
      `容器版本、发布说明与下载入口请前往 [GitHub Releases](${repositoryUrl}/releases)。`,
    ];
  }

  if (trackName === "cli") {
    return [
      "Public release notes for Luna CLI. The CLI works independently; each new release must include the exact same version of the Luna DevOps Skill in the same GitHub Release.",
      "",
      "The current development line binds the CLI and Skill to one version, tag, and release.",
      "",
      `Install or download releases from [GitHub Releases](${repositoryUrl}/releases).`,
    ];
  }
  if (trackName === "cli-skills") {
    return [
      "Public release notes for the Luna DevOps Skill. New Skill versions are published together with the exact same Luna CLI version.",
      "",
      "Only one `luna-devops` Skill is published. Its root `SKILL.md` routes tasks and loads domain guidance from `references/` on demand. The Skill requires the exact same Luna CLI version, while historical standalone Skills releases remain available for traceability.",
      "",
      `Download \`luna-devops-<version>.skill\` from [GitHub Releases](${repositoryUrl}/releases).`,
    ];
  }
  return [
    "Public release notes for the Luna DevOps platform.",
    "",
    `Find container releases and release notes on [GitHub Releases](${repositoryUrl}/releases).`,
  ];
}

function generatePage(trackName, track, lang) {
  const tags = trackTags(trackName, track);
  const lines = [
    `# ${track.title}${lang === "zh" ? " 更新日志" : " Changelog"}`,
    "",
    ...pageIntroduction(trackName, lang),
    "",
  ];

  if (tags.length === 0) {
    lines.push(
      lang === "zh" ? "暂时还没有公开版本。" : "No public releases yet.",
      "",
    );
  } else {
    tags.forEach((tag, index) => {
      lines.push(
        releaseSection(trackName, track, tag, tags[index + 1] ?? "", lang),
      );
    });
  }

  const output = join(root, "docs", "docs", lang, "changelog", track.output);
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, `${lines.join("\n").trim()}\n`);
}

function selectedTracks(argv) {
  const argument = argv.find(value => value.startsWith("--track="));
  if (!argument || argument === "--track=all") return Object.keys(tracks);
  const name = argument.slice("--track=".length);
  if (!tracks[name]) {
    throw new Error(`Unknown changelog track: ${name}`);
  }
  return [name];
}

for (const lang of ["zh", "en"]) {
  const oldFile = join(root, "docs", "docs", lang, "changelog.md");
  rmSync(oldFile, { force: true });
  for (const trackName of selectedTracks(process.argv.slice(2))) {
    generatePage(trackName, tracks[trackName], lang);
  }
}
