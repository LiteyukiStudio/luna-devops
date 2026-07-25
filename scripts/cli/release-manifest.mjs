import {
  readdirSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";

import {
  fail,
  isMainModule,
  parseArguments,
  readJson,
  repositoryRoot,
  requiredArgument,
  sha256,
} from "./lib.mjs";

const GENERATED_FILES = new Set([
  "RELEASE-MANIFEST.json",
  "RELEASE_NOTES.md",
  "SHA256SUMS",
]);

function releaseFiles(directory) {
  return readdirSync(directory)
    .filter(name => !GENERATED_FILES.has(name))
    .filter(name => statSync(join(directory, name)).isFile())
    .sort();
}

export function generateReleaseManifest({
  directory,
  tag,
  version,
  commit,
  prerelease,
  npmTag,
}) {
  const root = resolve(directory);
  const files = releaseFiles(root).map((name) => ({
    name,
    size: statSync(join(root, name)).size,
    sha256: sha256(join(root, name)),
  }));
  const unsigned = files
    .filter(file => file.name.includes("-unsigned"))
    .map(file => file.name);

  const checksums = files
    .map(file => `${file.sha256}  ${file.name}`)
    .join("\n");
  writeFileSync(join(root, "SHA256SUMS"), `${checksums}\n`);

  const manifest = {
    schemaVersion: 1,
    product: "Luna CLI",
    tag,
    version,
    commit,
    prerelease,
    npmTag,
    recommended: {
      lunaCliSkills: readJson(
        join(repositoryRoot, "release-compatibility.json"),
      ).cli.recommendedSkills,
    },
    files,
    verification: {
      checksums: "SHA256SUMS",
      githubOidcAttestations: true,
      unsignedDesktopArtifacts: unsigned,
      stableDesktopArtifactsPublished: false,
    },
  };
  writeFileSync(
    join(root, "RELEASE-MANIFEST.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );

  const channel = prerelease ? "预发布 / Prerelease" : "正式版 / Stable";
  const recommendedSkills = manifest.recommended.lunaCliSkills;
  const notes = `# Luna CLI ${version}

${channel}

## 中文

- npm：\`npm install --global @liteyuki/luna-cli@${npmTag}\`
- 建议配套 Luna CLI Skills：\`${recommendedSkills}\`。CLI 可以不安装 Skills 独立使用。
- Linux 独立二进制已在目标 runner 完成 smoke test。
- macOS 尚未接入代码签名；仅预发布版本提供带 \`-unsigned\` 后缀的测试制品，正式版不发布这些制品。
- Windows 与 Alpine/musl 请通过 npm 或 pnpm 安装，并使用 Node.js 22.14.0 或更高版本运行。
- 请使用 \`SHA256SUMS\` 校验下载文件，并在 GitHub Release 的 Attestations 页面验证 OIDC provenance。

## English

- npm: \`npm install --global @liteyuki/luna-cli@${npmTag}\`
- Recommended Luna CLI Skills: \`${recommendedSkills}\`. The CLI works without Skills.
- Standalone Linux binaries were smoke-tested on their target runners.
- macOS code signing is not configured. Only prereleases contain explicitly named \`-unsigned\` test artifacts; stable releases omit them.
- On Windows and Alpine/musl, install with npm or pnpm and run the CLI on Node.js 22.14.0 or later.
- Verify downloads with \`SHA256SUMS\` and check the GitHub OIDC provenance on the release Attestations page.
`;
  writeFileSync(join(root, "RELEASE_NOTES.md"), notes);
  return manifest;
}

async function main() {
  const args = parseArguments(process.argv.slice(2));
  const manifest = generateReleaseManifest({
    directory: requiredArgument(args, "directory"),
    tag: requiredArgument(args, "tag"),
    version: requiredArgument(args, "version"),
    commit: requiredArgument(args, "commit"),
    prerelease: args.get("prerelease") === "true",
    npmTag: requiredArgument(args, "npm-tag"),
  });
  process.stdout.write(`${JSON.stringify(manifest, null, 2)}\n`);
}

if (isMainModule(import.meta.url)) {
  main().catch(fail);
}
