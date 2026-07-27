import { mkdtempSync, readFileSync, readdirSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const skillsRoot = join(root, "ai-supports", "skills");
const lunaHome = mkdtempSync(join(tmpdir(), "luna-skills-sync-"));
const errors = [];
const localProtocolCategories = new Set(["api", "completion", "help", "version"]);
const categoryReferences = new Map([
  ["access-token", "security.md"],
  ["app-templates", "deployment.md"],
  ["application", "deployment.md"],
  ["auth", "security.md"],
  ["billing", "billing.md"],
  ["build", "build.md"],
  ["cluster", "runtime.md"],
  ["config", "system.md"],
  ["dashboard", "workspace.md"],
  ["data-retention", "system.md"],
  ["deployment", "deployment.md"],
  ["event", "debugging.md"],
  ["events", "debugging.md"],
  ["gateway", "gateway.md"],
  ["git", "source.md"],
  ["health", "debugging.md"],
  ["notification", "notifications.md"],
  ["notifications", "notifications.md"],
  ["o-auth-applications", "security.md"],
  ["project", "workspace.md"],
  ["registry", "registry.md"],
  ["release", "deployment.md"],
  ["releases", "deployment.md"],
  ["retention", "system.md"],
  ["runtime", "runtime.md"],
  ["system", "system.md"],
  ["topology", "topology.md"],
  ["user", "security.md"],
]);
const staleClaimPatterns = [
  {
    pattern: /尚未进入\s*CLI/gu,
    message: "must discover capabilities from machine Help instead of declaring them absent",
  },
  {
    pattern: /当前命令目录没有/gu,
    message: "must not freeze a missing-category snapshot in Skill text",
  },
  {
    pattern: /本引用只用于(?:定义|分析|整理|规划)/gu,
    message: "must describe the dynamic CLI workflow instead of planning-only behavior",
  },
  {
    pattern: /这些(?:流程|能力).{0,24}(?:只能|仅能)(?:分析|规划)/gu,
    message: "must not retain planning-only capability assertions",
  },
  {
    pattern: /当前\s*`?[a-z0-9-]+`?\s*分类(?:只覆盖|只有)/gu,
    message: "must not freeze partial category coverage in prose",
  },
  {
    pattern: /当前可执行能力是/gu,
    message: "must not freeze a version-specific executable capability list",
  },
  {
    pattern: /(?:Device Code|Step-up MFA|OAuth Bearer Step-up MFA).{0,40}(?:未实现|不支持)/gu,
    message: "contains an obsolete authentication capability assertion",
  },
  {
    pattern: /(?:CLI|命令目录).{0,24}(?:尚无|缺少|未实现|不支持).{0,24}(?:通知|计费|账单|Gateway|网关|构建|发布|部署|运行时|拓扑)/giu,
    message: "contains an obsolete major-domain capability assertion",
  },
];

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
    "output=json",
    "interactive=false",
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
  const items = [];
  let cursor;
  do {
    const envelope = runCatalogPage(cursor);
    for (const item of envelope.data.items) {
      items.push(item);
    }
    cursor = envelope.data.nextCursor;
  } while (cursor);
  return items;
}

function frontmatterName(source) {
  const frontmatter = source.match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1];
  return frontmatter?.match(/^name:\s*([^\r\n]+)$/m)?.[1]?.trim();
}

function markdownFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return markdownFiles(path);
    }
    return entry.isFile() && entry.name.endsWith(".md") ? [path] : [];
  });
}

function validateAgentCommands(path, source, commandPaths) {
  const displayPath = relative(root, path);
  const commandPattern = /\bluna\s+([a-z0-9-]+)\s+([a-z0-9-]+)([^\r\n`]*)/g;
  for (const match of source.matchAll(commandPattern)) {
    const commandPath = `${match[1]}.${match[2]}`;
    const argumentsText = match[3];
    if (!commandPaths.has(commandPath)) {
      errors.push(`${displayPath}: command "${commandPath}" is not present in machine Help`);
    }
    if (!/\bagent=true\b/.test(argumentsText)) {
      errors.push(`${displayPath}: Agent command "${commandPath}" must include agent=true`);
    }
    if (!/\boutput=json\b/.test(argumentsText)) {
      errors.push(`${displayPath}: Agent command "${commandPath}" must include output=json`);
    }
    if (!/(?:\binteractive=false\b|--no-interactive\b)/.test(argumentsText)) {
      errors.push(
        `${displayPath}: Agent command "${commandPath}" must disable interactive input`,
      );
    }
  }
}

function validateStaleClaims(path, source) {
  const displayPath = relative(root, path);
  for (const claim of staleClaimPatterns) {
    claim.pattern.lastIndex = 0;
    if (claim.pattern.test(source)) {
      errors.push(`${displayPath}: ${claim.message}`);
    }
  }
}

function validateSkill(path, commandPaths) {
  const source = readFileSync(path, "utf8");
  const skillDirectory = basename(dirname(path));
  const displayPath = relative(root, path);
  const name = frontmatterName(source);

  if (name !== skillDirectory) {
    errors.push(`${displayPath}: frontmatter name must equal directory name "${skillDirectory}"`);
  }
  if (skillDirectory !== "luna-devops") {
    errors.push(`${displayPath}: only ai-supports/skills/luna-devops may contain SKILL.md`);
  }

  const referencePattern = /\]\(references\/([a-z0-9-]+\.md)\)/g;
  const references = new Set(
    [...source.matchAll(referencePattern)].map(match => match[1]),
  );
  const referenceDirectory = join(dirname(path), "references");
  const availableReferences = new Set(
    readdirSync(referenceDirectory)
      .filter(name => name.endsWith(".md")),
  );
  for (const reference of references) {
    if (!availableReferences.has(reference)) {
      errors.push(`${displayPath}: missing domain reference "${reference}"`);
    }
  }
  for (const reference of availableReferences) {
    if (!references.has(reference)) {
      errors.push(`${displayPath}: unreferenced domain file "references/${reference}"`);
    }
  }

  for (const markdownPath of markdownFiles(dirname(path))) {
    const markdownSource = readFileSync(markdownPath, "utf8");
    validateAgentCommands(markdownPath, markdownSource, commandPaths);
    validateStaleClaims(markdownPath, markdownSource);
  }
}

function validateCategoryCoverage(catalog, referenceDirectory) {
  const categories = new Set(catalog.map(item => item.category));
  for (const category of categories) {
    if (localProtocolCategories.has(category)) {
      continue;
    }
    const reference = categoryReferences.get(category);
    if (!reference) {
      errors.push(
        `machine Help category "${category}" has no progressive domain reference mapping`,
      );
      continue;
    }
    try {
      if (!statSync(join(referenceDirectory, reference)).isFile()) {
        errors.push(
          `machine Help category "${category}" maps to missing reference "${reference}"`,
        );
      }
    } catch {
      errors.push(
        `machine Help category "${category}" maps to missing reference "${reference}"`,
      );
    }
  }
}

try {
  const catalog = loadCommandCatalog();
  const commandPaths = new Set(catalog.map(item => item.path));
  const skillFile = join(skillsRoot, "luna-devops", "SKILL.md");
  validateSkill(skillFile, commandPaths);
  validateCategoryCoverage(catalog, join(dirname(skillFile), "references"));

  if (errors.length > 0) {
    console.error("Luna DevOps Skill sync check failed:");
    for (const error of errors) {
      console.error(`- ${error}`);
    }
    process.exitCode = 1;
  } else {
    const coveredCategories = new Set(
      catalog
        .map(item => item.category)
        .filter(category => !localProtocolCategories.has(category)),
    );
    console.log(
      `Luna CLI Skill sync check passed: 1 skill, ${coveredCategories.size} covered categories, ${commandPaths.size} commands.`,
    );
  }
} finally {
  rmSync(lunaHome, { recursive: true, force: true });
}
