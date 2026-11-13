<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import { useStreamText } from '../composables/useStreamText'

const props = defineProps<{ text: string; status: string; externalLinksOnly?: boolean }>()
const root = ref<HTMLElement>()
const selected = ref(false)
const text = useStreamText(() => props.text, () => props.status, selected)
const markdown = new MarkdownIt({ html: false, linkify: true, breaks: true })
const blocks = computed(() => {
  const environment = {}
  const tokens = markdown.parse(text.value, environment)
  if (props.externalLinksOnly) {
    for (const token of tokens) {
      const linkTags: string[] = []
      for (const child of token.children ?? []) {
        if (child.type === 'link_open') {
          const external = /^https?:\/\//i.test(child.attrGet('href') ?? '')
          child.tag = external ? 'a' : 'span'
          linkTags.push(child.tag)
          if (external) {
            child.attrSet('target', '_blank')
            child.attrSet('rel', 'noopener noreferrer')
          } else child.attrs = null
        } else if (child.type === 'link_close') child.tag = linkTags.pop() ?? 'span'
      }
    }
  }
  const result: string[] = []
  let start = 0
  for (let index = 0; index < tokens.length; index++) {
    const token = tokens[index]!
    if (token.level === 0 && token.nesting !== 1) {
      result.push(DOMPurify.sanitize(markdown.renderer.render(tokens.slice(start, index + 1), markdown.options, environment), { FORBID_TAGS: ['img', 'style'], ADD_ATTR: props.externalLinksOnly ? ['target'] : [] }))
      start = index + 1
    }
  }
  return result
})
function trackSelection() {
  const selection = document.getSelection()
  selected.value = Boolean(selection && !selection.isCollapsed &&
    (root.value?.contains(selection.anchorNode) || root.value?.contains(selection.focusNode)))
}
document.addEventListener('selectionchange', trackSelection)
onBeforeUnmount(() => document.removeEventListener('selectionchange', trackSelection))
</script>

<template>
  <div ref="root" class="markdown" :class="{ streaming: status === 'running' || status === 'inProgress' }">
    <div v-for="(block, index) in blocks" :key="index" class="markdown-block" v-html="block"></div>
  </div>
</template>
