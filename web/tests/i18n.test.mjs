import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { parse as parseComponent } from '@vue/compiler-sfc'
import { baseParse } from '@vue/compiler-dom'
import { en, zhCN } from '../src/i18n/catalog.ts'
import { languageStorageKey, readPreference, resolveLocale, savePreference, translate, translateError, translateMessage, translateSessionTitle, translateState } from '../src/i18n/locale.ts'
import { synchronizeDocumentLanguage, useI18n } from '../src/i18n/index.ts'
import { nextTick } from 'vue'
import { shouldSendTask } from '../src/composables/composerKeyboard.ts'

test('automatic language selection follows the first supported browser preference', () => {
  assert.equal(resolveLocale(['zh-CN', 'en-US']), 'zh-CN')
  assert.equal(resolveLocale(['en-GB', 'zh-CN']), 'en')
  assert.equal(resolveLocale(['de-DE', 'zh-Hant-TW', 'en']), 'zh-CN')
  assert.equal(resolveLocale(['ZH_hans']), 'zh-CN')
  assert.equal(resolveLocale(['fr-FR']), 'en')
  assert.equal(resolveLocale([]), 'en')
})

test('saved languages persist and legacy system choices use initial detection', () => {
  const values = new Map()
  const storage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value) }
  assert.equal(readPreference(storage), undefined)
  savePreference('en', storage)
  assert.equal(values.get(languageStorageKey), 'en')
  assert.equal(readPreference(storage), 'en')
  savePreference('zh-CN', storage)
  assert.equal(readPreference(storage), 'zh-CN')
  values.set(languageStorageKey, 'system')
  assert.equal(readPreference(storage), undefined)
  values.set(languageStorageKey, 'invalid')
  assert.equal(readPreference(storage), undefined)
})

test('blocked browser storage does not prevent choosing a language', () => {
  const storage = { getItem() { throw new Error('Storage blocked') }, setItem() { throw new Error('Storage blocked') } }
  assert.equal(readPreference(storage), undefined)
  assert.doesNotThrow(() => savePreference('zh-CN', storage))
})

test('switching the shared locale updates text and statuses without restarting the workspace', () => {
  const i18n = useI18n()
  i18n.setLanguage('zh-CN')
  assert.equal(i18n.t('connection.connections'), '连接')
  assert.equal(i18n.stateLabel('waiting_approval'), '等待审批')
  i18n.setLanguage('en')
  assert.equal(i18n.t('connection.connections'), 'Connections')
  assert.equal(i18n.stateLabel('waiting_approval'), 'Waiting for approval')
  i18n.setLanguage('system')
  assert.equal(i18n.locale.value, 'en')
})

test('document language and metadata follow manual, browser, and cross-tab changes', async () => {
  const descriptors = Object.fromEntries(['window', 'document', 'navigator'].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)]))
  const values = new Map([[languageStorageKey, 'system']])
  const fakeWindow = new EventTarget()
  fakeWindow.localStorage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value) }
  const metadata = {}
  const fakeDocument = { documentElement: { lang: '' }, title: '', querySelector: () => ({ setAttribute: (name, value) => metadata[name] = value }) }
  const fakeNavigator = { languages: ['zh-CN', 'en-US'], language: 'zh-CN' }
  for (const [name, value] of [['window', fakeWindow], ['document', fakeDocument], ['navigator', fakeNavigator]]) {
    Object.defineProperty(globalThis, name, { value, configurable: true })
  }
  const i18n = useI18n()
  const stop = synchronizeDocumentLanguage()
  try {
    fakeWindow.dispatchEvent(Object.assign(new Event('storage'), { key: null }))
    fakeWindow.dispatchEvent(new Event('languagechange'))
    await nextTick()
    assert.equal(fakeDocument.documentElement.lang, 'zh-CN')
    assert.equal(fakeDocument.title, 'Loom — 智能体工作区')
    assert.equal(metadata.content, 'Loom，本地编程智能体工作区。')
    i18n.setLanguage('en')
    fakeWindow.dispatchEvent(new Event('languagechange'))
    await nextTick()
    assert.equal(fakeDocument.documentElement.lang, 'en')
    assert.equal(values.get(languageStorageKey), 'en')
    values.clear()
    fakeWindow.dispatchEvent(Object.assign(new Event('storage'), { key: null }))
    await nextTick()
    assert.equal(fakeDocument.documentElement.lang, 'zh-CN')
    values.set(languageStorageKey, 'en')
    const event = new Event('storage')
    Object.defineProperty(event, 'key', { value: languageStorageKey })
    fakeWindow.dispatchEvent(event)
    await nextTick()
    assert.equal(i18n.preference.value, 'en')
    assert.equal(fakeDocument.title, 'Loom — Agent workspace')
  } finally {
    values.clear()
    fakeWindow.dispatchEvent(Object.assign(new Event('storage'), { key: null }))
    stop()
    for (const [name, descriptor] of Object.entries(descriptors)) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor)
      else delete globalThis[name]
    }
  }
})

test('every interface message has both translations with the same placeholders', () => {
  assert.deepEqual(Object.keys(zhCN).sort(), Object.keys(en).sort())
  for (const key of Object.keys(en)) {
    assert.ok(en[key].trim(), `empty English translation: ${key}`)
    assert.ok(zhCN[key].trim(), `empty Chinese translation: ${key}`)
    const placeholders = value => [...value.matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort()
    assert.deepEqual(placeholders(zhCN[key]), placeholders(en[key]), key)
  }
  assert.equal(translate('zh-CN', 'discovery.progress', { checked: 12, total: 20 }), '已检查 12 / 20')
  assert.equal(translateState('zh-CN', 'inProgress'), '正在执行')
})

test('controller-owned errors localize while upstream diagnostics remain verbatim', () => {
  assert.equal(translateError('zh-CN', { code: 'not_found', message: 'Service not found.' }, 'errors.action'), '未找到服务。')
  assert.equal(translateMessage('zh-CN', 'Codex handshake failed: authentication required: update the service token'), 'Codex 握手失败：需要身份验证，请更新服务令牌。')
  assert.equal(translateMessage('zh-CN', 'Task acknowledgement was lost. Synchronize the session; this task will not be resubmitted automatically.'), '未收到任务确认，请同步会话。此任务不会自动重复提交。')
  assert.equal(translateMessage('zh-CN', 'persist controller state: database is locked'), '无法保存工作区状态：database is locked')
  assert.equal(translateMessage('zh-CN', 'connection closed'), '智能体连接已关闭。')
  assert.equal(translateMessage('zh-CN', 'context canceled'), '请求已取消。')
  assert.equal(translateMessage('zh-CN', 'Model diagnostic: quota for custom-provider'), 'Model diagnostic: quota for custom-provider')
  assert.equal(translateError('zh-CN', undefined, 'errors.reply'), '无法发送回答。')
})

test('generated session titles translate without rewriting user-defined names', () => {
  assert.equal(translateSessionTitle('zh-CN', { title: 'New session', defaultTitle: true }), '新建会话')
  assert.equal(translateSessionTitle('en', { title: 'New session', defaultTitle: true }), 'New session')
  assert.equal(translateSessionTitle('zh-CN', { title: 'New session', defaultTitle: false }), 'New session')
  assert.equal(translateSessionTitle('zh-CN', { title: 'My feature' }), 'My feature')
  assert.equal(translateSessionTitle('zh-CN', { title: 'New session' }), '新建会话')
})

test('confirming Chinese input and inserting a newline do not send a task', () => {
  const enter = { key: 'Enter', isComposing: false, keyCode: 13, shiftKey: false, altKey: false, ctrlKey: false, metaKey: false }
  assert.equal(shouldSendTask({ ...enter, isComposing: true }), false)
  assert.equal(shouldSendTask({ ...enter, keyCode: 229 }), false)
  assert.equal(shouldSendTask({ ...enter, shiftKey: true }), false)
  assert.equal(shouldSendTask(enter), true)
})

test('visible Vue text and accessible attributes use the translation catalog', () => {
  const files = ['App.vue', ...readdirSync(new URL('../src/components/', import.meta.url)).filter(name => name.endsWith('.vue')).map(name => `components/${name}`), 'views/Workspace.vue']
  const untranslated = []
  function visit(node, file) {
    if (node.type === 2 && /[A-Za-z\u3400-\u9fff]/.test(node.content) && node.content.trim() !== 'Loom') {
      untranslated.push(`${file}: ${node.content.trim()}`)
    }
    if (node.type === 1) {
      for (const prop of node.props) {
        if (prop.type === 6 && ['aria-label', 'title', 'placeholder', 'alt'].includes(prop.name) && prop.value && !/^wss?:\/\//.test(prop.value.content)) {
          untranslated.push(`${file}: ${prop.name}=${prop.value.content}`)
        }
      }
    }
    for (const child of node.children ?? []) visit(child, file)
  }
  for (const file of files) {
    const { descriptor } = parseComponent(readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8'))
    if (descriptor.template) visit(baseParse(descriptor.template.content), file)
  }
  assert.deepEqual(untranslated, [])
})
