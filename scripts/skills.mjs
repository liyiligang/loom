import { createHash } from 'node:crypto'
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, rmdirSync, unlinkSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const projectRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(new URL('../web/package.json', import.meta.url))
const { parseDocument } = require('yaml')
const textExtensions = new Set(['.md', '.yaml', '.yml', '.json', '.mjs', '.js', '.ts', '.py', '.sh', '.ps1', '.txt', '.html', '.css'])
const digest = value => createHash('sha256').update(value).digest('hex')

function confined(root, relative) {
  if (!relative || relative.includes('\\') || path.posix.isAbsolute(relative) || relative.split('/').some(part => !part || part === '.' || part === '..')) {
    throw new Error(`Invalid owned path: ${relative}`)
  }
  const file = path.resolve(root, ...relative.split('/'))
  let current = root
  for (const part of ['.', ...relative.split('/')]) {
    current = path.resolve(current, part)
    if (existsSync(current) && lstatSync(current).isSymbolicLink()) throw new Error(`Linked skill paths cannot be synchronized: ${current}`)
  }
  return file
}

function bytes(file) {
  const raw = readFileSync(file)
  if (!textExtensions.has(path.extname(file))) return raw
  const text = new TextDecoder('utf-8', { fatal: true }).decode(raw)
  return Buffer.from(text.replace(/\r\n/g, '\n'))
}

function filesUnder(root, directories = new Set()) {
  if (!existsSync(root)) return new Map()
  if (lstatSync(root).isSymbolicLink()) throw new Error(`Linked skill root cannot be synchronized: ${root}`)
  const files = new Map()
  function walk(relative = '') {
    for (const entry of readdirSync(path.join(root, relative), { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name, 'en'))) {
      const name = relative ? `${relative}/${entry.name}` : entry.name
      const file = confined(root, name)
      if (entry.isDirectory()) {
        directories.add(name)
        walk(name)
      }
      else if (entry.isFile()) files.set(name, bytes(file))
      else throw new Error(`Unsupported skill asset: ${name}`)
    }
  }
  walk()
  return files
}

function yaml(source, file) {
  const doc = parseDocument(source, { uniqueKeys: true })
  if (doc.errors.length) throw new Error(`${file}: ${doc.errors[0].message}`)
  const value = doc.toJS()
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${file}: YAML must be a mapping.`)
  return value
}

export function validateSource(root = projectRoot) {
  const sourceRoot = confined(root, '.agents/skills')
  const files = filesUnder(sourceRoot)
  let skills = 0
  for (const [name, data] of files) {
    if (path.posix.basename(name) === 'SKILL.md') {
      const source = data.toString('utf8').replace(/^\uFEFF/, '')
      const frontmatter = /^---\n([\s\S]*?)\n---(?:\n|$)/.exec(source)
      if (!frontmatter) throw new Error(`${name}: SKILL.md requires YAML front matter.`)
      const metadata = yaml(frontmatter[1], name)
      if (typeof metadata.name !== 'string' || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(metadata.name)
        || metadata.name.length > 64 || metadata.name !== path.posix.basename(path.posix.dirname(name))) throw new Error(`${name}: Invalid skill identifier.`)
      if (typeof metadata.description !== 'string' || !metadata.description.trim()) throw new Error(`${name}: Missing description.`)
      if (metadata['disable-model-invocation'] !== undefined && typeof metadata['disable-model-invocation'] !== 'boolean') throw new Error(`${name}: Invocation setting must be boolean.`)
      const policyName = `${path.posix.dirname(name)}/agents/openai.yaml`
      const config = files.has(policyName) ? yaml(files.get(policyName).toString('utf8'), policyName) : {}
      const implicit = config.policy?.allow_implicit_invocation
      if (implicit !== undefined && typeof implicit !== 'boolean') throw new Error(`${policyName}: Invocation policy must be boolean.`)
      if ((metadata['disable-model-invocation'] === true) !== (implicit === false)) throw new Error(`${name}: Claude and Codex invocation policies disagree.`)
      skills++
    }
    if (name.endsWith('.md')) {
      const prose = data.toString('utf8').replace(/^(```|~~~)[\s\S]*?^\1[^\n]*$/gm, '')
      for (const match of prose.matchAll(/\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)/g)) {
        const target = match[1].split('#')[0]
        if (!target || /^[a-z][a-z0-9+.-]*:/i.test(target) || target.startsWith('/')) continue
        if (!existsSync(path.resolve(path.dirname(path.join(sourceRoot, name)), decodeURIComponent(target)))) throw new Error(`${name}: Missing reference ${target}`)
      }
    }
  }
  if (!skills) throw new Error('No canonical skills found.')
  return { files, skills }
}

function lockAt(root) {
  const file = confined(root, 'scripts/skills-lock.json')
  const lock = JSON.parse(readFileSync(file, 'utf8'))
  if (lock.schemaVersion !== 1 || !lock.files || typeof lock.files !== 'object' || Array.isArray(lock.files)) throw new Error('Invalid skill synchronization lock.')
  for (const [name, value] of Object.entries(lock.files)) {
    confined(confined(root, '.claude/skills'), name)
    if (typeof value !== 'string' || !/^[a-f0-9]{64}$/.test(value)) throw new Error(`Invalid skill hash: ${name}`)
  }
  return { file, lock }
}

export function checkSkills(root = projectRoot) {
  const { files, skills } = validateSource(root)
  const { lock } = lockAt(root)
  const mirror = filesUnder(confined(root, '.claude/skills'))
  for (const [name, data] of files) {
    if (lock.files[name] !== digest(data) || !mirror.has(name) || digest(mirror.get(name)) !== digest(data)) throw new Error(`Skill mirror is stale: ${name}. Run node scripts/skills.mjs sync.`)
  }
  for (const name of Object.keys(lock.files)) if (!files.has(name)) throw new Error(`Generated skill file lost its source: ${name}. Run sync.`)
  return { skills, files: files.size, unownedMirrorFiles: [...mirror.keys()].filter(name => !Object.hasOwn(lock.files, name)).length }
}

export function syncSkills(root = projectRoot) {
  const { files, skills } = validateSource(root)
  const { file: lockFile, lock } = lockAt(root)
  const mirrorRoot = confined(root, '.claude/skills')
  const mirrorDirectories = new Set()
  const mirror = filesUnder(mirrorRoot, mirrorDirectories)
  const ownedDirectories = new Set()
  for (const name of Object.keys(lock.files)) {
    for (let parent = path.posix.dirname(name); parent !== '.'; parent = path.posix.dirname(parent)) ownedDirectories.add(parent)
  }
  const owned = new Set([...Object.keys(lock.files), ...files.keys()])
  for (const name of owned) {
    if (!mirror.has(name)) continue
    const current = digest(mirror.get(name))
    if (current !== lock.files[name] && (!files.has(name) || current !== digest(files.get(name)))) throw new Error(`Independent mirror edit preserved: ${name}. Reconcile it into .agents/skills before syncing.`)
  }
  for (const name of files.keys()) {
    const parts = name.split('/')
    for (let i = 1; i < parts.length; i++) {
      const parent = parts.slice(0, i).join('/')
      if (mirror.has(parent) && !Object.hasOwn(lock.files, parent)) throw new Error(`Unowned mirror file blocks synchronization: ${parent}`)
    }
    const destination = confined(mirrorRoot, name)
    if (existsSync(destination) && lstatSync(destination).isDirectory()) {
      const children = [...mirror.keys()].filter(child => child.startsWith(`${name}/`))
      const directories = [...mirrorDirectories].filter(child => child === name || child.startsWith(`${name}/`))
      if (directories.some(child => !ownedDirectories.has(child)) || children.some(child => !Object.hasOwn(lock.files, child))) {
        throw new Error(`Unowned mirror directory blocks synchronization: ${name}`)
      }
    }
  }
  const next = {}
  let updated = 0
  let removed = 0
  const obsoleteDirectories = new Set()
  for (const name of Object.keys(lock.files)) {
    if (!files.has(name)) {
      for (let parent = path.posix.dirname(name); parent !== '.'; parent = path.posix.dirname(parent)) obsoleteDirectories.add(parent)
      if (!mirror.has(name)) continue
      const destination = confined(mirrorRoot, name)
      unlinkSync(destination)
      removed++
    }
  }
  for (const name of [...obsoleteDirectories].sort((a, b) => b.split('/').length - a.split('/').length)) {
    const directory = confined(mirrorRoot, name)
    if (!existsSync(directory) || !lstatSync(directory).isDirectory()) continue
    try { rmdirSync(directory) } catch (error) {
      if (!['ENOTEMPTY', 'EEXIST', 'ENOENT'].includes(error.code)) throw error
    }
  }
  for (const [name, data] of files) {
    next[name] = digest(data)
    if (mirror.has(name) && digest(mirror.get(name)) === next[name]) continue
    const destination = confined(mirrorRoot, name)
    mkdirSync(path.dirname(destination), { recursive: true })
    writeFileSync(destination, data)
    updated++
  }
  writeFileSync(lockFile, `${JSON.stringify({ schemaVersion: 1, files: next }, null, 2)}\n`)
  return { skills, files: files.size, updated, removed }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const command = process.argv[2]
    if (process.argv.length !== 3 || !['sync', 'check'].includes(command)) throw new Error('Usage: node scripts/skills.mjs sync|check')
    const result = command === 'sync' ? syncSkills() : checkSkills()
    process.stdout.write(`${JSON.stringify(result)}\n`)
  } catch (error) {
    process.stderr.write(`${error.message}\n`)
    process.exitCode = 1
  }
}
