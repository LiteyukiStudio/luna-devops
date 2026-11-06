// 把公开文档复制进 Agent 包，供 search_docs / read_doc 检索；文档版本随 Agent 构建固定，不在运行时访问文档站。
import { createHash } from "node:crypto"
import { cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { dirname, join, relative } from "node:path"
import { fileURLToPath } from "node:url"

const packageRoot = join(dirname(fileURLToPath(import.meta.url)), "..")
const sourceRoot = join(packageRoot, "..", "docs", "docs")
const targetRoot = join(packageRoot, "knowledge", "docs")
const locales = ["zh", "en"]

if (!existsSync(sourceRoot))
  throw new Error(`docs source not found: ${sourceRoot}`)

rmSync(targetRoot, { force: true, recursive: true })
const files: string[] = []
const navigation: string[] = []
const digest = createHash("sha256")
for (const locale of locales) {
  const localeRoot = join(sourceRoot, locale)
  for (const file of docsFiles(localeRoot)) {
    const relativePath = relative(sourceRoot, file).split("\\").join("/")
    const destination = join(targetRoot, relativePath)
    mkdirSync(dirname(destination), { recursive: true })
    cpSync(file, destination)
    digest.update(relativePath).update("\0").update(readFileSync(file)).update("\0")
    if (file.endsWith("_meta.json") || file.endsWith("_nav.json")) navigation.push(relativePath)
    else files.push(relativePath)
  }
}
if (!files.length)
  throw new Error(`no markdown docs found under ${sourceRoot}`)

writeFileSync(join(targetRoot, "manifest.json"), `${JSON.stringify({ digest: digest.digest("hex").slice(0, 16), files, navigation }, null, 2)}\n`)
console.log(`bundled ${files.length} docs into ${relative(packageRoot, targetRoot)}`)

// Markdown 正文、各目录 _meta.json 与语言根目录 _nav.json（文档站导航顺序和标签），供 list_docs 按目录树浏览。
function docsFiles(directory: string): string[] {
  return readdirSync(directory).sort().flatMap((name) => {
    const path = join(directory, name)
    if (statSync(path).isDirectory())
      return docsFiles(path)
    return /\.mdx?$/u.test(name) || name === "_meta.json" || name === "_nav.json" ? [path] : []
  })
}
