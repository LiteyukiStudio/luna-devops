import { createReadStream, createWriteStream } from 'node:fs'
import { readdir, rm, stat } from 'node:fs/promises'
import { extname, join, relative, resolve } from 'node:path'
import process from 'node:process'
import { pipeline } from 'node:stream/promises'
import { createBrotliCompress, createGzip, constants as zlibConstants } from 'node:zlib'

const webRoot = resolve(import.meta.dirname, '..')
const distRoot = resolve(webRoot, process.argv[2] ?? 'dist')
const compressibleExtensions = new Set(['.css', '.js', '.json', '.svg', '.txt', '.xml'])
const minimumCompressionSize = 1024

const files = await listFiles(distRoot)
let compressedFiles = 0

for (const file of files) {
  if (!compressibleExtensions.has(extname(file)))
    continue
  if ((await stat(file)).size < minimumCompressionSize)
    continue

  await Promise.all([
    compress(file, `${file}.br`, createBrotliCompress({
      params: {
        [zlibConstants.BROTLI_PARAM_QUALITY]: 11,
      },
    })),
    compress(file, `${file}.gz`, createGzip({ level: 9 })),
  ])
  compressedFiles += 1
}

console.log(`Optimized ${compressedFiles} static assets in ${relative(process.cwd(), distRoot) || 'dist'}`)

async function listFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const output = []
  for (const entry of entries) {
    const entryPath = join(directory, entry.name)
    if (entry.isDirectory())
      output.push(...await listFiles(entryPath))
    else if (entry.isFile() && !entry.name.endsWith('.br') && !entry.name.endsWith('.gz'))
      output.push(entryPath)
  }
  return output
}

async function compress(source, destination, transform) {
  await pipeline(createReadStream(source), transform, createWriteStream(destination))
  const [sourceInfo, destinationInfo] = await Promise.all([stat(source), stat(destination)])
  if (destinationInfo.size >= sourceInfo.size)
    await rm(destination, { force: true })
}
