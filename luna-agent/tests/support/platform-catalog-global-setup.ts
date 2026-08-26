import { execFileSync } from "node:child_process"
import { mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import type { TestProject } from "vitest/node"

declare module "vitest" {
  export interface ProvidedContext {
    platformCatalogFixturePath: string
  }
}

export default function setup(project: TestProject): () => void {
  const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..")
  const fixtureDirectory = mkdtempSync(join(tmpdir(), "luna-agent-platform-catalog-"))
  const fixturePath = join(fixtureDirectory, "platform-catalog.json")
  const fixture = execFileSync("go", [
    "run",
    "./internal/aitool/testdata/export_platform_catalog.go",
  ], {
    cwd: repositoryRoot,
    maxBuffer: 2 * 1024 * 1024,
  })

  writeFileSync(fixturePath, fixture)
  project.provide("platformCatalogFixturePath", fixturePath)

  return () => rmSync(fixtureDirectory, { force: true, recursive: true })
}
