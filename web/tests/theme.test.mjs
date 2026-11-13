import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { nextTick } from 'vue'
import { readThemePreference, resolveTheme, saveThemePreference, themeStorageKey } from '../src/composables/themePreference.ts'
import { synchronizeDocumentTheme, useTheme } from '../src/composables/useTheme.ts'

test('a saved theme overrides the system; missing, invalid, or blocked storage follows the system', () => {
  const values = new Map()
  const storage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value) }
  assert.equal(readThemePreference(storage), 'system')
  assert.equal(resolveTheme('system', true), 'dark')
  assert.equal(resolveTheme('system', false), 'light')
  saveThemePreference('light', storage)
  assert.equal(readThemePreference(storage), 'light')
  assert.equal(resolveTheme(readThemePreference(storage), true), 'light')
  saveThemePreference('dark', storage)
  assert.equal(resolveTheme(readThemePreference(storage), false), 'dark')
  values.set(themeStorageKey, 'invalid')
  assert.equal(readThemePreference(storage), 'system')
  const blocked = { getItem() { throw Error('blocked') }, setItem() { throw Error('blocked') } }
  assert.equal(readThemePreference(blocked), 'system')
  assert.doesNotThrow(() => saveThemePreference('dark', blocked))
})

test('the external startup script applies the saved or system theme before application mounting', () => {
  const source = readFileSync(new URL('../public/theme.js', import.meta.url), 'utf8')
  for (const [saved, systemDark, expected] of [
    ['light', true, 'light'], ['dark', false, 'dark'], [null, true, 'dark'], ['invalid', false, 'light'], ['blocked', true, 'dark'],
  ]) {
    const root = { dataset: {}, classList: { toggle(name, value) { root[name] = value } } }
    runInNewContext(source, {
      localStorage: { getItem() { if (saved === 'blocked') throw Error('blocked'); return saved } },
      matchMedia: () => ({ matches: systemDark }),
      document: { documentElement: root },
    })
    assert.equal(root.dataset.theme, expected)
    assert.equal(root.dark, expected === 'dark')
  }
})

test('system changes, manual selection, and cross-tab preferences update the document consistently', async () => {
  const descriptors = Object.fromEntries(['window', 'document'].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)]))
  const values = new Map()
  const media = new EventTarget()
  media.matches = true
  const fakeWindow = new EventTarget()
  fakeWindow.localStorage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value) }
  fakeWindow.matchMedia = () => media
  const root = { dataset: {}, classList: { toggle(name, value) { root[name] = value } } }
  const metadata = {}
  const fakeDocument = { documentElement: root, querySelector: () => ({ setAttribute: (key, value) => metadata[key] = value }) }
  Object.defineProperty(globalThis, 'window', { value: fakeWindow, configurable: true })
  Object.defineProperty(globalThis, 'document', { value: fakeDocument, configurable: true })
  const controller = useTheme()
  const stop = synchronizeDocumentTheme()
  try {
    assert.equal(root.dataset.theme, 'dark')
    assert.equal(root.dark, true)
    assert.equal(metadata.content, '#25282e')
    media.matches = false
    media.dispatchEvent(new Event('change'))
    await nextTick()
    assert.equal(root.dataset.theme, 'light')
    controller.setTheme('dark')
    await nextTick()
    assert.equal(values.get(themeStorageKey), 'dark')
    assert.equal(root.dataset.theme, 'dark')
    media.matches = true
    media.dispatchEvent(new Event('change'))
    media.matches = false
    media.dispatchEvent(new Event('change'))
    await nextTick()
    assert.equal(root.dataset.theme, 'dark')
    values.set(themeStorageKey, 'light')
    const event = new Event('storage')
    Object.defineProperty(event, 'key', { value: themeStorageKey })
    fakeWindow.dispatchEvent(event)
    await nextTick()
    assert.equal(root.dataset.theme, 'light')
    assert.equal(root.dark, false)
    assert.equal(metadata.content, '#ffffff')
    values.clear()
    fakeWindow.dispatchEvent(event)
    media.matches = true
    media.dispatchEvent(new Event('change'))
    await nextTick()
    assert.equal(controller.preference.value, 'system')
    assert.equal(root.dataset.theme, 'dark')
  } finally {
    stop()
    controller.setTheme('system')
    for (const [name, descriptor] of Object.entries(descriptors)) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor)
      else delete globalThis[name]
    }
  }
})
