import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export function checkGoFormat(cwd = process.cwd()) {
  const options = { cwd, encoding: 'utf8', timeout: 30000, stdio: ['ignore', 'pipe', 'pipe'] }
  const listed = execFileSync('git', ['ls-files', '--cached', '--others', '--exclude-standard', '-z', '--', '*.go'], options)
  const files = [...new Set(listed.split('\0').filter(name => name && existsSync(path.resolve(cwd, name))))].sort()
  const unformatted = []
  for (let start = 0; start < files.length; start += 100) {
    const output = execFileSync('gofmt', ['-l', '--', ...files.slice(start, start + 100)], options)
    if (output.trim()) unformatted.push(output.trim())
  }
  if (unformatted.length) throw new Error(`Unformatted Go files:\n${unformatted.join('\n')}`)
  return { files: files.length }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.stdout.write(`${JSON.stringify(checkGoFormat())}\n`)
  } catch (error) {
    process.stderr.write(`${error.message}\n`)
    process.exitCode = 1
  }
}
