import { isMap, isScalar, parseDocument } from 'yaml'

// Keep the creation form's name authoritative without replacing instructions.
export function withSkillName(source: string, name: string): string {
  const frontMatter = /^(\uFEFF?[ \t]*---[ \t]*\r?\n)([\s\S]*?)(^[ \t]*---[ \t]*(?:\r?\n|$))/m.exec(source)
  if (!frontMatter || frontMatter.index !== 0) {
    if (/^\uFEFF?[ \t]*---[ \t]*(?:\r?\n|$)/.test(source)) return source
    const bom = source.startsWith('\uFEFF') ? '\uFEFF' : ''
    return `${bom}---\nname: new-skill\nmetadata:\n  display_name: ${JSON.stringify(name)}\n---\n\n${source.slice(bom.length)}`
  }
  if (frontMatter[2]!.length > (16 << 10)) return source
  const document = parseDocument(frontMatter[2]!)
  if (document.errors.length || document.contents !== null && !isMap(document.contents)) return source
  const metadata = document.get('metadata', true)
  if (isScalar(metadata) && metadata.value === null) document.set('metadata', document.createNode({}))
  else if (metadata != null && !isMap(metadata)) return source
  if (document.getIn(['metadata', 'display_name']) === name) return source
  document.setIn(['metadata', 'display_name'], name)
  const newline = frontMatter[1]!.endsWith('\r\n') ? '\r\n' : '\n'
  const header = document.toString().replace(/\r?\n/g, newline)
  return `${frontMatter[1]}${header}${frontMatter[3]}${source.slice(frontMatter[0].length)}`
}
