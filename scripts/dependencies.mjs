import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const prefix = process.argv[2]
if (process.argv.length !== 3 || !['web', 'bridge/claude'].includes(prefix)) {
  process.stderr.write('Usage: node scripts/dependencies.mjs web|bridge/claude\n')
  process.exitCode = 2
} else {
  try {
    const directory = path.join(root, prefix)
    const manifest = JSON.parse(readFileSync(path.join(directory, 'package.json'), 'utf8'))
    const lock = JSON.parse(readFileSync(path.join(directory, 'package-lock.json'), 'utf8'))
    const names = Object.keys({ ...manifest.dependencies, ...manifest.devDependencies })
    const ready = names.every(name => {
      const file = path.join(directory, 'node_modules', name, 'package.json')
      return existsSync(file) && JSON.parse(readFileSync(file, 'utf8')).version === lock.packages[`node_modules/${name}`]?.version
    })
    process.exitCode = ready ? 0 : 1
  } catch {
    process.exitCode = 1
  }
}
