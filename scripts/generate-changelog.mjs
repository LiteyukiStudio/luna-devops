import {
  mkdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { execFileSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const repositoryUrl = "https://github.com/LiteyukiStudio/luna-devops";
const tagPattern = "v[0-9]*";
const outputFile = "index.md";
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

function git(args) {
  return execFileSync("git", args, {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
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
  return value
    .replaceAll("\\", "\\\\")
    .replaceAll("[", "\\[")
    .replaceAll("]", "\\]");
}

function releaseTags() {
  const output = git(["tag", "--list", tagPattern, "--sort=-v:refname"]);
  return output ? output.split("\n").filter(Boolean) : [];
}

function commitsForRange(range) {
  const output = git([
    "log",
    "--no-merges",
    "--format=%H%x1f%s",
    range,
  ]);
  if (!output) return [];
  return output.split("\n").map((line) => {
    const [hash, subject] = line.split("\x1f");
    return { hash, subject, category: categoryForSubject(subject) };
  });
}

function releaseSection(tag, previousTag, lang) {
  const version = tag.slice(1);
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

  const commits = commitsForRange(range);
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
        ? "此版本没有提交记录。"
        : "No commits matched this release.",
      "",
    );
  }
  return lines.join("\n");
}

function generatePage(lang) {
  const tags = releaseTags();
  const lines = [
    `# Luna DevOps${lang === "zh" ? " 更新日志" : " Changelog"}`,
    "",
    lang === "zh"
      ? "这里记录 Luna DevOps 平台本体的公开版本变化。"
      : "Public release notes for the Luna DevOps platform.",
    "",
    lang === "zh"
      ? `容器版本、发布说明与下载入口请前往 [GitHub Releases](${repositoryUrl}/releases)。`
      : `Find container releases and release notes on [GitHub Releases](${repositoryUrl}/releases).`,
    "",
  ];

  if (tags.length === 0) {
    lines.push(
      lang === "zh" ? "暂时还没有公开版本。" : "No public releases yet.",
      "",
    );
  } else {
    tags.forEach((tag, index) => {
      lines.push(releaseSection(tag, tags[index + 1] ?? "", lang));
    });
  }

  const output = join(root, "docs", "docs", lang, "changelog", outputFile);
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, `${lines.join("\n").trim()}\n`);
}

for (const lang of ["zh", "en"]) {
  rmSync(join(root, "docs", "docs", lang, "changelog.md"), { force: true });
  generatePage(lang);
}
