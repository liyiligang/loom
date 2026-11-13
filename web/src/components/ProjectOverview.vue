<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { BookOpen, KeyRound, Cpu } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { Endpoint, ModelCatalog, Project, ProjectSkill, Session } from '../types'
import ProjectCredentials from './ProjectCredentials.vue'
import ProjectSkills from './ProjectSkills.vue'
import ModelSettings from './ModelSettings.vue'

const props = defineProps<{ catalog?: ModelCatalog; project: Project; session?: Session; refreshKey: number; busy: boolean; endpoints: Endpoint[]; refresh: () => Promise<void> }>()
const emit = defineEmits<{
  manageModels: []
  openSkill: [skill: ProjectSkill]
  save: [update: { secretIds: string[] }]
}>()
const { t } = useI18n()
type Tab = 'skills' | 'credentials' | 'models'
const tab = ref<Tab>('skills')
const credentialsVisited = ref(false)
const secretIds = ref([...(props.project.secretIds ?? [])])
const credentialsLoading = ref(false)
const modelBusy = ref(false)
const skillsButton = ref<HTMLButtonElement>()
const credentialsButton = ref<HTMLButtonElement>()
const modelsButton = ref<HTMLButtonElement>()
const scopeId = `project-${props.project.id}-${props.session?.id ?? 'root'}`

function selectTab(value: Tab, focus = false) {
  if (props.busy || modelBusy.value) return
  tab.value = value
  if (value === 'credentials') credentialsVisited.value = true
  if (focus) void nextTick(() => (value === 'skills' ? skillsButton.value : value === 'credentials' ? credentialsButton.value : modelsButton.value)?.focus())
}
function navigateTabs(event: KeyboardEvent) {
  let value: Tab
  if (event.key === 'Home') value = 'skills'
  else if (event.key === 'End') value = 'models'
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
    const values: Tab[] = ['skills', 'credentials', 'models']
    value = values[(values.indexOf(tab.value) + (event.key === 'ArrowRight' ? 1 : values.length - 1)) % values.length]!
  }
  else return
  event.preventDefault()
  selectTab(value, true)
}
function save() {
  if (!props.busy && !credentialsLoading.value) emit('save', { secretIds: secretIds.value })
}
</script>

<template>
  <section class="project-overview">
    <div class="content-tabs" role="tablist" :aria-label="t('navigation.projectTabs')" @keydown="navigateTabs">
      <button :id="`${scopeId}-skills-tab`" ref="skillsButton" role="tab" :aria-selected="tab === 'skills'" :aria-controls="`${scopeId}-skills-panel`" :tabindex="tab === 'skills' ? 0 : -1" :disabled="busy || modelBusy" @click="selectTab('skills')"><BookOpen :size="15" aria-hidden="true" />{{ t('skills.title') }}</button>
      <button :id="`${scopeId}-credentials-tab`" ref="credentialsButton" role="tab" :aria-selected="tab === 'credentials'" :aria-controls="`${scopeId}-credentials-panel`" :tabindex="tab === 'credentials' ? 0 : -1" :disabled="busy || modelBusy" @click="selectTab('credentials')"><KeyRound :size="15" aria-hidden="true" />{{ t('secrets.bindProjectCredentials') }}</button>
      <button :id="`${scopeId}-models-tab`" ref="modelsButton" role="tab" :aria-selected="tab === 'models'" :aria-controls="`${scopeId}-models-panel`" :tabindex="tab === 'models' ? 0 : -1" :disabled="busy || modelBusy" @click="selectTab('models')"><Cpu :size="15" aria-hidden="true" />{{ t('models.title') }}</button>
    </div>
    <div :id="`${scopeId}-skills-panel`" v-show="tab === 'skills'" role="tabpanel" :aria-labelledby="`${scopeId}-skills-tab`" :inert="tab !== 'skills'" tabindex="0">
      <ProjectSkills :project="project" :session="session" @open="emit('openSkill', $event)" />
    </div>
    <div :id="`${scopeId}-credentials-panel`" v-show="tab === 'credentials'" role="tabpanel" :aria-labelledby="`${scopeId}-credentials-tab`" :inert="tab !== 'credentials'" tabindex="0">
      <form v-if="credentialsVisited" class="settings-form" novalidate @submit.prevent="save">
        <ProjectCredentials v-model="secretIds" :project-id="project.id" :local="project.local" :refresh-key="refreshKey" :disabled="busy" @loading="credentialsLoading = $event" />
        <button class="button primary" :disabled="busy || credentialsLoading">{{ t(busy ? 'secrets.saving' : 'navigation.save') }}</button>
      </form>
    </div>
    <div v-if="tab === 'models'" :id="`${scopeId}-models-panel`" role="tabpanel" :aria-labelledby="`${scopeId}-models-tab`" tabindex="0"><ModelSettings :catalog="catalog" @manage-models="emit('manageModels')" :project="project" :endpoints="endpoints" :disabled="busy" :refresh="refresh" @busy="modelBusy = $event" /></div>
  </section>
</template>
