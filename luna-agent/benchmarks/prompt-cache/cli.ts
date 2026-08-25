import { mkdir, readFile, writeFile } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import process from "node:process"
import {
  failedPromptCacheBenchmarkAssertions,
  isPromptCacheBenchmarkResult,
  runPromptCacheBenchmark,
  type PromptCacheBenchmarkResult,
} from "./benchmark.js"
import {
  comparePromptCacheBenchmarks,
  isPromptCacheBenchmarkComparison,
} from "./comparison.js"
import { renderPromptCacheBenchmarkReport } from "./report.js"

// ContextCompiler 会输出正常运行日志；benchmark 的 stdout 只保留机器可读结果。
process.env.LOG_LEVEL ??= "error"

const usage = `用法：
  pnpm benchmark:prompt-cache -- --label baseline --output result.json [--report report.html]
  pnpm benchmark:prompt-cache:compare -- --baseline baseline.json --optimized optimized.json --output comparison.json [--report report.html]
  pnpm benchmark:prompt-cache:report -- --input result-or-comparison.json --output report.html

约定：
  --output -  写入 stdout（benchmark/compare 默认值）
  --input -   从 stdin 读取（report 可用）
  所有场景均为本地固定 fixture，不发起 Provider 网络请求。`

void main().catch((error: unknown) => {
  process.stderr.write(`prompt-cache benchmark failed: ${error instanceof Error ? error.message : String(error)}\n`)
  process.exitCode = 1
})

async function main(): Promise<void> {
  const [command, ...rawArgs] = process.argv.slice(2)
  const args = rawArgs[0] === "--" ? rawArgs.slice(1) : rawArgs
  if (!command || command === "--help" || command === "-h") {
    process.stdout.write(`${usage}\n`)
    return
  }
  if (command === "run") return runCommand(args)
  if (command === "compare") return compareCommand(args)
  if (command === "report") return reportCommand(args)
  throw new Error(`unknown_command:${command}\n${usage}`)
}

async function runCommand(args: string[]): Promise<void> {
  const options = parseOptions(args, ["label", "output", "report"])
  const result = await runPromptCacheBenchmark({
    ...(options.label ? { checkoutLabel: options.label } : {}),
  })
  await writeText(options.output ?? "-", `${JSON.stringify(result, null, 2)}\n`)
  if (options.report) await writeText(options.report, renderPromptCacheBenchmarkReport(result))
  const failures = failedPromptCacheBenchmarkAssertions(result)
  if (failures.length) {
    for (const failure of failures) {
      process.stderr.write(`FAIL ${failure.scenarioId}/${failure.assertion.id}: ${failure.assertion.description}\n`)
    }
    process.exitCode = 1
  }
}

async function compareCommand(args: string[]): Promise<void> {
  const options = parseOptions(args, ["baseline", "optimized", "output", "report"])
  const baseline = await readBenchmarkResult(requiredOption(options, "baseline"))
  const optimized = await readBenchmarkResult(requiredOption(options, "optimized"))
  const comparison = comparePromptCacheBenchmarks(baseline, optimized)
  await writeText(options.output ?? "-", `${JSON.stringify(comparison, null, 2)}\n`)
  if (options.report) await writeText(options.report, renderPromptCacheBenchmarkReport(comparison))
  if (!comparison.summary.functionalAssertionsPassed) process.exitCode = 1
}

async function reportCommand(args: string[]): Promise<void> {
  const options = parseOptions(args, ["input", "output"])
  const input = await readJSON(requiredOption(options, "input"))
  if (!isPromptCacheBenchmarkResult(input) && !isPromptCacheBenchmarkComparison(input))
    throw new Error("prompt_cache_benchmark_input_invalid")
  await writeText(options.output ?? "-", renderPromptCacheBenchmarkReport(input))
}

async function readBenchmarkResult(path: string): Promise<PromptCacheBenchmarkResult> {
  const value = await readJSON(path)
  if (!isPromptCacheBenchmarkResult(value)) throw new Error(`prompt_cache_benchmark_result_invalid:${path}`)
  return value
}

async function readJSON(path: string): Promise<unknown> {
  const text = path === "-" ? await readStdin() : await readFile(resolve(path), "utf8")
  return JSON.parse(text) as unknown
}

async function readStdin(): Promise<string> {
  let text = ""
  for await (const chunk of process.stdin as AsyncIterable<unknown>) {
    if (typeof chunk === "string") text += chunk
    else if (chunk instanceof Uint8Array) text += Buffer.from(chunk).toString("utf8")
    else throw new Error("prompt_cache_benchmark_stdin_invalid")
  }
  return text
}

async function writeText(path: string, value: string): Promise<void> {
  if (path === "-") {
    process.stdout.write(value)
    return
  }
  const absolutePath = resolve(path)
  await mkdir(dirname(absolutePath), { recursive: true })
  await writeFile(absolutePath, value, "utf8")
}

function parseOptions(args: string[], allowed: string[]): Record<string, string> {
  const options: Record<string, string> = {}
  for (let index = 0; index < args.length; index += 2) {
    const rawName = args[index]
    const value = args[index + 1]
    if (!rawName?.startsWith("--") || !value || value.startsWith("--"))
      throw new Error(`invalid_option:${rawName ?? "missing"}`)
    const name = rawName.slice(2)
    if (!allowed.includes(name)) throw new Error(`unknown_option:${rawName}`)
    if (options[name] !== undefined) throw new Error(`duplicate_option:${rawName}`)
    options[name] = value
  }
  return options
}

function requiredOption(options: Record<string, string>, name: string): string {
  const value = options[name]
  if (!value) throw new Error(`missing_option:--${name}`)
  return value
}
