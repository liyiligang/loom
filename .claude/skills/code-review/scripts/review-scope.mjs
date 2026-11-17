import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { lstatSync, readFileSync, readlinkSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const limit = 8 * 1024 * 1024
const hash = value => createHash('sha256').update(value).digest('hex')

export function captureReview({ cwd = process.cwd(), mode = 'workspace', base } = {}) {
  if (!['workspace', 'commits'].includes(mode)) throw new Error('Mode must be workspace or commits.')
  if (mode === 'commits' && !base) throw new Error('Commit review requires an explicit base reference.')
  const git = (...args) => execFileSync('git', args, {
    cwd, encoding: 'utf8', maxBuffer: limit, timeout: 30000, stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, GIT_OPTIONAL_LOCKS: '0' },
  })
  const directory = git('rev-parse', '--show-toplevel').trim()
  cwd = directory
  const requestedBase = base ?? 'HEAD'
  const baseCommit = git('rev-parse', '--verify', '--end-of-options', `${requestedBase}^{commit}`).trim()
  const headCommit = git('rev-parse', '--verify', 'HEAD^{commit}').trim()
  const mergeBaseCommit = git('merge-base', baseCommit, headCommit).trim()
  const diff = (...args) => git('diff', '--no-ext-diff', '--no-textconv', '--no-color', '--binary', ...args, '--')

  function inputs() {
    const trackedDiff = mode === 'commits' ? diff(mergeBaseCommit, headCommit) : diff(mergeBaseCommit)
    const stagedDiff = mode === 'workspace' ? diff('--cached', mergeBaseCommit) : ''
    const unstagedDiff = mode === 'workspace' ? diff() : ''
    const untracked = []
    let bytes = Buffer.byteLength(trackedDiff + stagedDiff + unstagedDiff)
    if (mode === 'workspace') {
      const names = git('ls-files', '--others', '--exclude-standard', '-z').split('\0').filter(Boolean).sort()
      for (const name of names) {
        const file = path.resolve(directory, name)
        const relative = path.relative(directory, file)
        if (relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) throw new Error('File escaped the review scope.')
        const stat = lstatSync(file)
        if (stat.isSymbolicLink()) {
          const target = readlinkSync(file)
          bytes += Buffer.byteLength(target)
          untracked.push({ path: name, kind: 'symlink', target, sha256: hash(target) })
        } else if (stat.isFile()) {
          if (stat.size > limit - bytes) throw new Error(`Review input is too large: ${name}`)
          const data = readFileSync(file)
          bytes += data.length
          let content = null
          if (!data.includes(0)) {
            try { content = new TextDecoder('utf-8', { fatal: true }).decode(data) } catch { /* Binary evidence is hashed. */ }
          }
          untracked.push({ path: name, kind: content === null ? 'binary' : 'text', size: data.length, sha256: hash(data), content })
        } else {
          throw new Error(`Unsupported new file: ${name}`)
        }
        if (bytes > limit) throw new Error('Review inputs exceed 8 MiB; narrow the task before claiming a complete review.')
      }
    }
    if (bytes > limit) throw new Error('Review inputs exceed 8 MiB; narrow the task before claiming a complete review.')
    return { trackedDiff, stagedDiff, unstagedDiff, untracked }
  }

  const captured = inputs()
  const snapshotHash = hash(JSON.stringify({ mode, baseCommit, mergeBaseCommit, headCommit, ...captured }))
  const current = inputs()
  if (snapshotHash !== hash(JSON.stringify({ mode, baseCommit, mergeBaseCommit, headCommit, ...current }))
    || git('rev-parse', '--verify', 'HEAD^{commit}').trim() !== headCommit) {
    throw new Error('Review inputs changed during capture; retry after the writer finishes.')
  }
  return {
    schemaVersion: 1, directory, mode, requestedBase, baseCommit, mergeBaseCommit, headCommit, snapshotHash,
    empty: !captured.trackedDiff && !captured.stagedDiff && !captured.unstagedDiff && !captured.untracked.length,
    localChangesExcluded: mode === 'commits' && Boolean(git('status', '--porcelain', '-z')),
    commits: git('log', '--oneline', `${mergeBaseCommit}..${headCommit}`, '--').trim().split('\n').filter(Boolean),
    ...captured,
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const options = {}
    const args = process.argv.slice(2)
    for (let i = 0; i < args.length; i += 2) {
      const key = { '--cwd': 'cwd', '--mode': 'mode', '--base': 'base' }[args[i]]
      if (!key || !args[i + 1]) throw new Error('Usage: review-scope.mjs [--cwd directory] [--mode workspace|commits] [--base reference]')
      options[key] = args[i + 1]
    }
    process.stdout.write(`${JSON.stringify(captureReview(options), null, 2)}\n`)
  } catch (error) {
    process.stderr.write(`${error.message}\n`)
    process.exitCode = 1
  }
}
