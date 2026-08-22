#!/usr/bin/env node

import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { performance } from 'node:perf_hooks'
import process from 'node:process'
import { fileURLToPath, pathToFileURL } from 'node:url'
import ts from 'typescript'

const startedAt = performance.now()
const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const srcRoot = path.join(webRoot, 'src')
const localesRoot = path.join(srcRoot, 'i18n', 'locales')
const appEntryPath = path.join(srcRoot, 'App.tsx')
const configModule = await import(pathToFileURL(path.join(srcRoot, 'i18n', 'config.ts')).href)
const dynamicKeyAllowlistModule = await import(pathToFileURL(path.join(webRoot, 'scripts', 'i18n-dynamic-key-allowlist.mjs')).href)
const auditedDynamicTranslationCalls = dynamicKeyAllowlistModule.auditedDynamicTranslationCalls
const supportedLanguages = [...configModule.supportedLanguages]
const coreTranslationBundles = new Set(configModule.coreTranslationBundles)
const spreadFeatureBundles = new Set(configModule.spreadFeatureBundles)
const baseLanguage = 'zh-CN'

const issues = []
const warnings = []
const issueSignatures = new Set()

function addIssue(category, message, signature = `${category}:${message}`) {
  if (issueSignatures.has(signature))
    return
  issueSignatures.add(signature)
  issues.push({ category, message })
}

function addWarning(category, message, signature) {
  warnings.push({ category, message, signature })
}

function relativePath(filePath) {
  return path.relative(webRoot, filePath).split(path.sep).join('/')
}

function flattenResources(value, prefix = '', target = new Map()) {
  if (typeof value === 'string') {
    if (prefix)
      target.set(prefix, value)
    return target
  }
  if (Array.isArray(value)) {
    if (prefix)
      target.set(prefix, JSON.stringify(value))
    return target
  }
  if (!value || typeof value !== 'object')
    return target

  for (const [key, child] of Object.entries(value))
    flattenResources(child, prefix ? `${prefix}.${key}` : key, target)
  return target
}

function interpolationTokens(value) {
  const tokens = new Set()
  let cursor = 0
  while (cursor < value.length) {
    const start = value.indexOf('{{', cursor)
    if (start < 0)
      break
    const end = value.indexOf('}}', start + 2)
    if (end < 0)
      break
    const token = value.slice(start + 2, end).split(',', 1)[0]?.trim()
    if (token)
      tokens.add(token)
    cursor = end + 2
  }
  return [...tokens].sort()
}

async function localeFileNames(language) {
  return (await readdir(path.join(localesRoot, language), { withFileTypes: true }))
    .filter(entry => entry.isFile() && entry.name.endsWith('.ts'))
    .map(entry => entry.name.replace(/\.ts$/, ''))
    .sort()
}

async function importLocaleBundle(language, bundleName) {
  const filePath = path.join(localesRoot, language, `${bundleName}.ts`)
  const fileStat = await stat(filePath)
  const moduleURL = `${pathToFileURL(filePath).href}?mtime=${fileStat.mtimeMs}`
  const module = await import(moduleURL)
  const resources = bundleName === 'root' || spreadFeatureBundles.has(bundleName)
    ? module.default
    : { [bundleName]: module.default }
  return flattenResources(resources)
}

async function loadCatalogs() {
  const filesByLanguage = new Map()
  for (const language of supportedLanguages)
    filesByLanguage.set(language, await localeFileNames(language))

  const baseFiles = filesByLanguage.get(baseLanguage) ?? []
  for (const language of supportedLanguages) {
    const languageFiles = new Set(filesByLanguage.get(language) ?? [])
    for (const bundleName of baseFiles) {
      if (!languageFiles.has(bundleName))
        addIssue('missing-locale-bundle', `${language}/${bundleName}.ts is missing; ${baseLanguage}/${bundleName}.ts exists.`)
    }
    for (const bundleName of languageFiles) {
      if (!baseFiles.includes(bundleName))
        addIssue('extra-locale-bundle', `${language}/${bundleName}.ts has no matching ${baseLanguage} bundle.`)
    }
  }

  const catalogs = new Map()
  for (const language of supportedLanguages) {
    const catalog = new Map()
    for (const bundleName of filesByLanguage.get(language) ?? []) {
      try {
        const bundle = await importLocaleBundle(language, bundleName)
        for (const [key, value] of bundle)
          catalog.set(key, value)
      }
      catch (error) {
        addIssue('invalid-locale-bundle', `${language}/${bundleName}.ts cannot be loaded: ${error instanceof Error ? error.message : String(error)}`)
      }
    }
    catalogs.set(language, catalog)
  }

  const lazyDirectory = path.join(srcRoot, 'i18n', 'lazy')
  const lazyResourceFiles = (await readdir(lazyDirectory, { withFileTypes: true }))
    .filter(entry => entry.isFile() && entry.name.endsWith('-resources.ts'))
    .map(entry => path.join(lazyDirectory, entry.name))
  for (const filePath of lazyResourceFiles) {
    try {
      const fileStat = await stat(filePath)
      const module = await import(`${pathToFileURL(filePath).href}?mtime=${fileStat.mtimeMs}`)
      const resourcePath = module.lazyTranslationResourcePath
      const resources = module.lazyTranslationResources
      if (typeof resourcePath !== 'string' || !resources || typeof resources !== 'object') {
        addIssue('invalid-lazy-resource', `${relativePath(filePath)} must export lazyTranslationResourcePath and lazyTranslationResources.`)
        continue
      }
      for (const language of supportedLanguages) {
        if (!resources[language]) {
          addIssue('missing-lazy-locale', `${relativePath(filePath)} has no resources for ${language}.`)
          continue
        }
        const flattened = flattenResources(resources[language], resourcePath)
        for (const [key, value] of flattened)
          catalogs.get(language)?.set(key, value)
      }
    }
    catch (error) {
      addIssue('invalid-lazy-resource', `${relativePath(filePath)} cannot be loaded: ${error instanceof Error ? error.message : String(error)}`)
    }
  }

  return { baseFiles, catalogs }
}

function checkLocaleParity(catalogs) {
  const allKeys = new Set()
  for (const catalog of catalogs.values()) {
    for (const key of catalog.keys())
      allKeys.add(key)
  }

  for (const key of [...allKeys].sort()) {
    const missingLanguages = supportedLanguages.filter(language => !catalogs.get(language)?.has(key))
    if (missingLanguages.length > 0) {
      addIssue(
        'locale-key-mismatch',
        `Key "${key}" is missing from: ${missingLanguages.join(', ')}.`,
        `locale-key-mismatch:${key}`,
      )
      continue
    }

    const tokensByLanguage = new Map(supportedLanguages.map(language => [
      language,
      interpolationTokens(catalogs.get(language)?.get(key) ?? ''),
    ]))
    const baseTokens = JSON.stringify(tokensByLanguage.get(baseLanguage))
    const mismatchedLanguages = supportedLanguages.filter(language => JSON.stringify(tokensByLanguage.get(language)) !== baseTokens)
    if (mismatchedLanguages.length > 0) {
      const details = supportedLanguages
        .map(language => `${language}=[${tokensByLanguage.get(language)?.join(', ') ?? ''}]`)
        .join('; ')
      addIssue('interpolation-mismatch', `Key "${key}" uses different interpolation variables: ${details}.`)
    }
  }
}

function sourceLocation(sourceFile, node) {
  const location = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile))
  return `${relativePath(sourceFile.fileName)}:${location.line + 1}:${location.character + 1}`
}

function translationPatterns(node) {
  if (ts.isStringLiteralLike(node))
    return [{ kind: 'exact', value: node.text }]
  if (ts.isTemplateExpression(node))
    return node.head.text ? [{ kind: 'prefix', value: node.head.text }] : []
  if (ts.isConditionalExpression(node))
    return [...translationPatterns(node.whenTrue), ...translationPatterns(node.whenFalse)]
  if (ts.isParenthesizedExpression(node))
    return translationPatterns(node.expression)
  if (ts.isBinaryExpression(node) && node.operatorToken.kind === ts.SyntaxKind.PlusToken) {
    const left = translationPatterns(node.left)[0]
    const right = translationPatterns(node.right)[0]
    if (left?.kind === 'exact' && right?.kind === 'exact')
      return [{ kind: 'exact', value: `${left.value}${right.value}` }]
    return left?.value ? [{ kind: 'prefix', value: left.value }] : []
  }
  return []
}

function isTranslationCall(node) {
  if (!ts.isCallExpression(node))
    return false
  if (ts.isIdentifier(node.expression))
    return node.expression.text === 't'
  return ts.isPropertyAccessExpression(node.expression)
    && ts.isIdentifier(node.expression.expression)
    && node.expression.expression.text === 'i18next'
    && node.expression.name.text === 't'
}

function hasDefaultValueOption(node) {
  const options = node.arguments[1]
  return Boolean(options && ts.isObjectLiteralExpression(options) && options.properties.some((property) => {
    return ts.isPropertyAssignment(property)
      && ((ts.isIdentifier(property.name) && property.name.text === 'defaultValue') || (ts.isStringLiteralLike(property.name) && property.name.text === 'defaultValue'))
  }))
}

function stringArrayArgument(node) {
  if (!node || !ts.isArrayLiteralExpression(node))
    return []
  return node.elements.filter(ts.isStringLiteralLike).map(element => element.text)
}

function scanSourceFile(filePath, sourceText) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    filePath.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  )
  const imports = []
  const references = []
  const declaredBundles = new Set()

  function visit(node) {
    if ((ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) && node.moduleSpecifier && ts.isStringLiteralLike(node.moduleSpecifier))
      imports.push(node.moduleSpecifier.text)

    if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword && node.arguments[0] && ts.isStringLiteralLike(node.arguments[0]))
      imports.push(node.arguments[0].text)

    if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && node.expression.text === 'loadTranslationBundles') {
      for (const bundleName of stringArrayArgument(node.arguments[0]))
        declaredBundles.add(bundleName)
    }

    if (isTranslationCall(node) && node.arguments[0]) {
      const patterns = translationPatterns(node.arguments[0])
      const hasDefaultValue = hasDefaultValueOption(node)
      if (patterns.length > 0) {
        for (const pattern of patterns)
          references.push({ ...pattern, hasDefaultValue, location: sourceLocation(sourceFile, node) })
      }
      else if (!hasDefaultValue) {
        const expression = node.arguments[0].getText(sourceFile)
        const signature = `${relativePath(sourceFile.fileName)}|${expression}`
        addWarning('unverifiable-dynamic-key', `${sourceLocation(sourceFile, node)} uses dynamic expression "${expression}". Audit signature: ${signature}`, signature)
      }
    }

    if (ts.isJsxAttribute(node) && node.name.getText(sourceFile) === 'i18nKey' && node.initializer && ts.isStringLiteral(node.initializer))
      references.push({ kind: 'exact', value: node.initializer.text, location: sourceLocation(sourceFile, node) })

    ts.forEachChild(node, visit)
  }

  visit(sourceFile)
  return { declaredBundles, imports, references, sourceFile }
}

async function listSourceFiles(directory) {
  const result = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const filePath = path.join(directory, entry.name)
    if (entry.isDirectory()) {
      if (filePath === localesRoot || entry.name === 'test')
        continue
      result.push(...await listSourceFiles(filePath))
      continue
    }
    if (!entry.isFile() || !/\.(?:ts|tsx)$/.test(entry.name) || /\.test\.(?:ts|tsx)$/.test(entry.name) || entry.name.endsWith('.d.ts'))
      continue
    result.push(filePath)
  }
  return result
}

async function scanSources() {
  const scans = new Map()
  for (const filePath of await listSourceFiles(srcRoot))
    scans.set(filePath, scanSourceFile(filePath, await readFile(filePath, 'utf8')))
  return scans
}

function bundleForPattern(pattern, featureBundles) {
  const normalized = pattern.replace(/[^\w.-].*$/, '')
  const bundleName = normalized.split('.')[0]
  return featureBundles.has(bundleName) ? bundleName : undefined
}

function checkReferencedKeys(scans, catalogs) {
  const allReferences = [...scans.values()].flatMap(scan => scan.references)
  for (const reference of allReferences) {
    if (reference.hasDefaultValue)
      continue
    const missingLanguages = supportedLanguages.filter((language) => {
      const catalog = catalogs.get(language) ?? new Map()
      if (reference.kind === 'exact')
        return !catalog.has(reference.value)
      return ![...catalog.keys()].some(key => key.startsWith(reference.value))
    })
    if (missingLanguages.length === 0)
      continue
    const label = reference.kind === 'exact' ? 'Key' : 'Dynamic key prefix'
    addIssue(
      'missing-key',
      `${label} "${reference.value}" used at ${reference.location} is missing from: ${missingLanguages.join(', ')}.`,
      `missing-key:${reference.location}:${reference.value}`,
    )
  }
}

function findLazyCall(node) {
  if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && ['lazyNamed', 'lazyTranslated'].includes(node.expression.text))
    return node
  let result
  ts.forEachChild(node, (child) => {
    if (!result)
      result = findLazyCall(child)
  })
  return result
}

function findImportedModule(node) {
  if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword && node.arguments[0] && ts.isStringLiteralLike(node.arguments[0]))
    return node.arguments[0].text
  let result
  ts.forEachChild(node, (child) => {
    if (!result)
      result = findImportedModule(child)
  })
  return result
}

function routeEntries(appScan) {
  const entries = []
  for (const statement of appScan.sourceFile.statements) {
    if (!ts.isVariableStatement(statement))
      continue
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name) || !declaration.initializer)
        continue
      const lazyCall = findLazyCall(declaration.initializer)
      if (!lazyCall)
        continue
      const moduleSpecifier = findImportedModule(lazyCall.arguments[0])
      if (!moduleSpecifier)
        continue
      entries.push({
        bundles: new Set(stringArrayArgument(lazyCall.arguments[2])),
        moduleSpecifier,
        name: declaration.name.text,
      })
    }
  }
  return entries
}

function resolveSourceImport(importerPath, moduleSpecifier, scans) {
  if (!moduleSpecifier.startsWith('.') && !moduleSpecifier.startsWith('@/'))
    return undefined
  const unresolvedPath = moduleSpecifier.startsWith('@/')
    ? path.join(srcRoot, moduleSpecifier.slice(2))
    : path.resolve(path.dirname(importerPath), moduleSpecifier)
  const candidates = [
    unresolvedPath,
    `${unresolvedPath}.ts`,
    `${unresolvedPath}.tsx`,
    path.join(unresolvedPath, 'index.ts'),
    path.join(unresolvedPath, 'index.tsx'),
  ]
  return candidates.find(candidate => scans.has(candidate))
}

function reachableFiles(entryPath, scans) {
  const visited = new Set()
  const pending = [entryPath]
  while (pending.length > 0) {
    const filePath = pending.pop()
    if (!filePath || visited.has(filePath))
      continue
    visited.add(filePath)
    const scan = scans.get(filePath)
    if (!scan)
      continue
    for (const moduleSpecifier of scan.imports) {
      const resolved = resolveSourceImport(filePath, moduleSpecifier, scans)
      if (resolved && !visited.has(resolved))
        pending.push(resolved)
    }
  }
  return visited
}

function checkRouteBundles(scans, featureBundles) {
  const appScan = scans.get(appEntryPath)
  if (!appScan) {
    addIssue('route-analysis-failed', 'src/App.tsx was not available to the i18n route analyzer.')
    return
  }
  const entries = routeEntries(appScan)
  const appLayout = entries.find(entry => entry.name === 'AppLayout')
  const globalBundles = appLayout?.bundles ?? new Set()

  for (const entry of entries) {
    const entryPath = resolveSourceImport(appEntryPath, entry.moduleSpecifier, scans)
    if (!entryPath) {
      addIssue('route-analysis-failed', `${entry.name} entry module "${entry.moduleSpecifier}" could not be resolved.`)
      continue
    }
    const loadedBundles = new Set(entry.bundles)
    if (entry.name !== 'AppLayout') {
      for (const bundleName of globalBundles)
        loadedBundles.add(bundleName)
    }

    const firstReferenceByBundle = new Map()
    for (const filePath of reachableFiles(entryPath, scans)) {
      const scan = scans.get(filePath)
      if (!scan)
        continue
      for (const bundleName of scan.declaredBundles)
        loadedBundles.add(bundleName)
      for (const reference of scan.references) {
        if (reference.hasDefaultValue)
          continue
        const bundleName = bundleForPattern(reference.value, featureBundles)
        if (bundleName && !firstReferenceByBundle.has(bundleName))
          firstReferenceByBundle.set(bundleName, reference)
      }
    }

    for (const [bundleName, reference] of firstReferenceByBundle) {
      if (loadedBundles.has(bundleName))
        continue
      addIssue(
        'missing-bundle',
        `${entry.name} references "${reference.value}" at ${reference.location}, but its route does not load the "${bundleName}" bundle.`,
        `missing-bundle:${entry.name}:${bundleName}`,
      )
    }
  }
}

function printDiagnostics() {
  const elapsed = performance.now() - startedAt
  if (issues.length === 0) {
    console.log(`i18n check passed in ${elapsed.toFixed(0)} ms.`)
  }
  else {
    console.error(`i18n check failed with ${issues.length} issue(s) in ${elapsed.toFixed(0)} ms.`)
    for (const issue of issues)
      console.error(`\n[${issue.category}] ${issue.message}`)
  }

  if (warnings.length > 0)
    console.log(`${warnings.length} audited dynamic translation call(s) use an explicit allowlist.`)
  console.log('Diagnostic categories: missing-bundle = resource exists but is not loaded; missing-key = referenced key does not exist; locale-key-mismatch = locales disagree.')
  if (issues.length > 0)
    process.exitCode = 1
}

const { baseFiles, catalogs } = await loadCatalogs()
checkLocaleParity(catalogs)
const scans = await scanSources()
const usedDynamicSignatures = new Set(warnings.map(warning => warning.signature))
for (const warning of warnings) {
  if (!auditedDynamicTranslationCalls[warning.signature])
    addIssue(warning.category, warning.message, `${warning.category}:${warning.signature}`)
}
for (const signature of Object.keys(auditedDynamicTranslationCalls)) {
  if (!usedDynamicSignatures.has(signature))
    addIssue('stale-dynamic-key-allowlist', `Remove or update unused dynamic translation audit entry: ${signature}`)
}
checkReferencedKeys(scans, catalogs)
const featureBundles = new Set(baseFiles.filter(bundleName => !coreTranslationBundles.has(bundleName)))
checkRouteBundles(scans, featureBundles)
printDiagnostics()
