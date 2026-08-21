import { readdir, readFile, stat } from 'node:fs/promises'
import { extname, join, relative, resolve } from 'node:path'
import process from 'node:process'
import { brotliCompressSync, gzipSync, constants as zlibConstants } from 'node:zlib'

const distRoot = resolve(process.argv[2] ?? 'dist')
const manifest = JSON.parse(await readFile(join(distRoot, '.vite/manifest.json'), 'utf8'))
const requestedSeeds = (process.argv[3] ?? 'index.html').split(',').filter(Boolean)
const files = await listFiles(distRoot)
const identityFiles = files.filter(file => !file.endsWith('.br') && !file.endsWith('.gz') && !file.includes(`${join(distRoot, '.vite')}/`))
const criticalFiles = collectManifestFiles(requestedSeeds)

const result = {
  criticalCss: await summarize(criticalFiles.filter(file => extname(file) === '.css')),
  criticalJavaScript: await summarize(criticalFiles.filter(file => extname(file) === '.js')),
  identityArtifactBytes: await sumFileSizes(identityFiles),
  packagedArtifactBytes: await sumFileSizes(files.filter(file => !file.includes(`${join(distRoot, '.vite')}/`))),
  repoOnlyPublicAssetBytes: await sumExisting([
    join(distRoot, 'brand/mascot-luna-devops.png'),
    join(distRoot, 'images/luna-devops-banner-v4.png'),
  ]),
  totalJavaScript: await summarize(identityFiles.filter(file => extname(file) === '.js')),
}

process.stdout.write(`${JSON.stringify(result, null, 2)}\n`)

function collectManifestFiles(seeds) {
  const collected = new Set()
  const visited = new Set()
  const visit = (key) => {
    if (visited.has(key))
      return
    visited.add(key)
    const entry = manifest[key]
    if (!entry)
      throw new Error(`Missing manifest entry: ${key}`)
    if (entry.file)
      collected.add(join(distRoot, entry.file))
    for (const cssFile of entry.css ?? [])
      collected.add(join(distRoot, cssFile))
    for (const imported of entry.imports ?? [])
      visit(imported)
  }
  for (const seed of seeds)
    visit(seed)
  return [...collected]
}

async function summarize(targetFiles) {
  const measurements = await Promise.all(targetFiles.map(async (file) => {
    const data = await readFile(file)
    const gzipPath = `${file}.gz`
    const brotliPath = `${file}.br`
    const gzipBytes = await existingSize(gzipPath) ?? gzipSync(data, { level: 9 }).byteLength
    const brotliBytes = await existingSize(brotliPath) ?? brotliCompressSync(data, {
      params: { [zlibConstants.BROTLI_PARAM_QUALITY]: 11 },
    }).byteLength
    return {
      brotliBytes,
      file: relative(distRoot, file),
      gzipBytes,
      rawBytes: data.byteLength,
      servedBytes: await existingSize(brotliPath) ?? await existingSize(gzipPath) ?? data.byteLength,
    }
  }))
  const largest = measurements.toSorted((left, right) => right.rawBytes - left.rawBytes)[0]
  return {
    brotliBytes: sum(measurements.map(item => item.brotliBytes)),
    fileCount: measurements.length,
    gzipBytes: sum(measurements.map(item => item.gzipBytes)),
    largestFile: largest?.file ?? '',
    largestRawBytes: largest?.rawBytes ?? 0,
    rawBytes: sum(measurements.map(item => item.rawBytes)),
    servedBytes: sum(measurements.map(item => item.servedBytes)),
  }
}

async function listFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const output = []
  for (const entry of entries) {
    const entryPath = join(directory, entry.name)
    if (entry.isDirectory())
      output.push(...await listFiles(entryPath))
    else if (entry.isFile())
      output.push(entryPath)
  }
  return output
}

async function existingSize(file) {
  try {
    return (await stat(file)).size
  }
  catch {
    return undefined
  }
}

async function sumExisting(targetFiles) {
  const sizes = await Promise.all(targetFiles.map(file => existingSize(file)))
  return sum(sizes.map(size => size ?? 0))
}

async function sumFileSizes(targetFiles) {
  const sizes = await Promise.all(targetFiles.map(async file => (await stat(file)).size))
  return sum(sizes)
}

function sum(values) {
  return values.reduce((total, value) => total + value, 0)
}
