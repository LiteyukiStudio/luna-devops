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
const visibleReleaseCount = 10;

function git(args) {
  return execFileSync("git", args, {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function releaseTags() {
  const output = git(["tag", "--list", tagPattern, "--sort=-v:refname"]);
  return output ? output.split("\n").filter(Boolean) : [];
}

function releaseLine(tag, lang) {
  const date = git(["log", "-1", "--format=%cs", tag]);
  const releaseUrl = `${repositoryUrl}/releases/tag/${encodeURIComponent(tag)}`;
  const label = lang === "zh" ? "查看发布说明" : "Release notes";
  return `- **${tag.slice(1)}** · ${date} · [${label}](${releaseUrl})`;
}

function generatePage(lang) {
  const tags = releaseTags();
  const lines = [
    `# Luna DevOps${lang === "zh" ? " 更新日志" : " Changelog"}`,
    "",
    lang === "zh"
      ? "这里只列出最近发布的版本。新增能力、修复和升级注意事项以对应的 GitHub Release 为准。"
      : "This page lists recent releases. See the matching GitHub Release for features, fixes, and upgrade notes.",
    "",
    lang === "zh"
      ? `查看[全部版本](${repositoryUrl}/releases)。`
      : `View [all releases](${repositoryUrl}/releases).`,
    "",
  ];

  if (tags.length === 0) {
    lines.push(
      lang === "zh" ? "暂时还没有公开版本。" : "No public releases yet.",
      "",
    );
  } else {
    lines.push("## " + (lang === "zh" ? "最近版本" : "Recent releases"), "");
    tags.slice(0, visibleReleaseCount).forEach((tag) => {
      lines.push(releaseLine(tag, lang));
    });
    lines.push("");
  }

  const output = join(root, "docs", "docs", lang, "changelog", outputFile);
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, `${lines.join("\n").trim()}\n`);
}

for (const lang of ["zh", "en"]) {
  rmSync(join(root, "docs", "docs", lang, "changelog.md"), { force: true });
  generatePage(lang);
}
