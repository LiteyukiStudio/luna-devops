import { execFileSync } from 'node:child_process'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const projectDirectory = fileURLToPath(new URL('..', import.meta.url))
const singletonPackages = [
  '@codemirror/commands',
  '@codemirror/language',
  '@codemirror/state',
  '@codemirror/view',
  'react',
  'react-dom',
]
const singletonPackageSet = new Set(singletonPackages)
const versionsByPackage = new Map(singletonPackages.map(packageName => [packageName, new Set()]))

const dependencyTrees = JSON.parse(execFileSync(
  'pnpm',
  ['list', ...singletonPackages, '--depth', 'Infinity', '--json'],
  { cwd: projectDirectory, encoding: 'utf8', maxBuffer: 10 * 1024 * 1024 },
))

for (const dependencyTree of dependencyTrees)
  collectDependencyVersions(dependencyTree)

const missingPackages = singletonPackages.filter(packageName => versionsByPackage.get(packageName)?.size === 0)
const duplicatePackages = singletonPackages.filter(packageName => (versionsByPackage.get(packageName)?.size ?? 0) > 1)

if (missingPackages.length > 0 || duplicatePackages.length > 0) {
  if (missingPackages.length > 0)
    console.error(`Missing singleton packages: ${missingPackages.join(', ')}`)
  for (const packageName of duplicatePackages) {
    const versions = [...versionsByPackage.get(packageName)].sort().join(', ')
    console.error(`Duplicate singleton package versions: ${packageName} (${versions})`)
  }
  console.error('Align the dependency family with pnpm-workspace.yaml overrides and regenerate pnpm-lock.yaml.')
  process.exit(1)
}

console.log(singletonPackages
  .map(packageName => `${packageName}@${[...versionsByPackage.get(packageName)][0]}`)
  .join('\n'))

function collectDependencyVersions(dependencyNode) {
  for (const dependencyGroup of ['dependencies', 'devDependencies', 'optionalDependencies']) {
    const dependencies = dependencyNode?.[dependencyGroup]
    if (!dependencies || typeof dependencies !== 'object')
      continue
    for (const [dependencyName, dependency] of Object.entries(dependencies)) {
      if (!dependency || typeof dependency !== 'object')
        continue
      if (singletonPackageSet.has(dependencyName) && typeof dependency.version === 'string')
        versionsByPackage.get(dependencyName).add(dependency.version)
      collectDependencyVersions(dependency)
    }
  }
}
