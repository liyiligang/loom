<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronRight, Folder, FolderOpen, GitBranch, Plus } from '@lucide/vue'
import type { Endpoint, Project, Requirement, Session } from '../types'
import { useI18n } from '../i18n'
import { navigationStorageKey, readExpansion, toggleExpansion } from '../composables/navigationExpansion'
import InlineName from './InlineName.vue'

const props = defineProps<{ projects: Project[]; requirements: Requirement[]; sessions: Session[]; endpoints: Endpoint[]; selectedId: string; activeProjectId?: string; activeRequirementId?: string; busy: boolean; rename: (kind: 'project' | 'requirement' | 'session', id: string, name: string) => Promise<void> }>()
const emit = defineEmits<{
  newRequirement: [project: Project]
  newSession: [requirement: Requirement]
  viewRequirement: [requirement: Requirement, section?: 'gitBranch']
  viewProject: [project: Project]
  navigate: []
}>()
const { t, sessionTitle, stateLabel } = useI18n()
function browserStorage() { try { return localStorage } catch { return undefined } }
const expansion = ref(readExpansion(browserStorage()))
const branches = computed(() => props.projects.map(project => ({
  project,
  requirements: props.requirements.filter(value => value.projectId === project.id).map(requirement => {
    const sessions = props.sessions.filter(value => value.requirementId === requirement.id && value.purpose !== 'requirement')
    return { requirement, sessions, mixedAgents: new Set(sessions.map(value => value.endpointId)).size > 1 }
  }),
})))
const selectedRequirement = computed(() => props.requirements.find(value => value.id === (props.activeRequirementId || props.sessions.find(session => session.id === props.selectedId)?.requirementId)))
const selectedProjectId = computed(() => props.activeProjectId || selectedRequirement.value?.projectId)
function toggle(key: string) { expansion.value = toggleExpansion(expansion.value, key) }
watch(expansion, value => { try { browserStorage()?.setItem(navigationStorageKey, JSON.stringify(value)) } catch { /* Navigation works without browser storage. */ } }, { flush: 'sync' })
</script>

<template>
  <nav class="project-tree" :aria-label="t('navigation.hierarchy')">
    <ul class="tree-root">
      <li v-for="branch in branches" :key="branch.project.id" class="project-node">
        <div class="tree-row project-row" :class="{ 'contains-selected': selectedProjectId === branch.project.id }">
          <button class="project-disclosure" :aria-label="t('skills.toggleProject', { name: branch.project.name })" :aria-expanded="Boolean(expansion[`project:${branch.project.id}`])" :aria-controls="`project-${branch.project.id}`" @click="toggle(`project:${branch.project.id}`)"><ChevronRight class="tree-chevron" :class="{ expanded: expansion[`project:${branch.project.id}`] }" :size="12" /></button>
          <InlineName v-slot="{ rename }" :name="branch.project.name" :label="t('navigation.renameProject', { name: branch.project.name })" :disabled="busy" :save="name => props.rename('project', branch.project.id, name)">
            <button class="project-directory" :aria-label="t('navigation.viewProject', { name: branch.project.name })" :aria-current="activeProjectId === branch.project.id && !activeRequirementId ? 'page' : undefined" :title="`${branch.project.cwd}\n${t('navigation.renameHint')}`" aria-keyshortcuts="F2" @click="emit('viewProject', branch.project)" @dblclick.stop="rename" @keydown.f2.stop.prevent="rename"><span class="project-folder" aria-hidden="true"><FolderOpen v-if="expansion[`project:${branch.project.id}`]" :size="15" /><Folder v-else :size="15" /></span><span class="tree-label">{{ branch.project.name }}</span></button>
          </InlineName>
          <button class="tree-action project-add" :aria-label="t('navigation.addRequirementTo', { name: branch.project.name })" :title="t('navigation.newRequirement')" :disabled="busy" @click="emit('newRequirement', branch.project)"><Plus :size="14" /></button>
        </div>
        <div :id="`project-${branch.project.id}`" class="tree-panel" :class="{ expanded: expansion[`project:${branch.project.id}`] }" :inert="!expansion[`project:${branch.project.id}`]">
          <div class="tree-panel-inner">
            <ul class="requirement-list">
              <li v-for="node in branch.requirements" :key="node.requirement.id">
                <div class="tree-row requirement-row" :class="{ 'contains-selected': selectedRequirement?.id === node.requirement.id }">
                  <button class="requirement-disclosure" :aria-label="t('navigation.toggleRequirement', { name: node.requirement.defaultName ? t('navigation.general') : node.requirement.name })" :aria-expanded="Boolean(expansion[`requirement:${node.requirement.id}`])" :aria-controls="`requirement-${node.requirement.id}`" @click="toggle(`requirement:${node.requirement.id}`)"><ChevronRight class="tree-chevron" :class="{ expanded: expansion[`requirement:${node.requirement.id}`] }" :size="11" /></button>
                  <InlineName v-slot="{ rename }" :name="node.requirement.defaultName ? t('navigation.general') : node.requirement.name" :label="t('navigation.renameRequirement', { name: node.requirement.defaultName ? t('navigation.general') : node.requirement.name })" :disabled="busy" :save="name => props.rename('requirement', node.requirement.id, name)">
                    <button class="requirement-directory" :aria-label="t('navigation.viewRequirement', { name: node.requirement.defaultName ? t('navigation.general') : node.requirement.name })" :aria-current="activeRequirementId === node.requirement.id ? 'page' : undefined" :title="t('navigation.renameHint')" aria-keyshortcuts="F2" @click="emit('viewRequirement', node.requirement)" @dblclick.stop="rename" @keydown.f2.stop.prevent="rename">
                      <span class="tree-label">{{ node.requirement.defaultName ? t('navigation.general') : node.requirement.name }}</span>
                    </button>
                  </InlineName>
                  <button class="tree-action requirement-add" :aria-label="t('navigation.addSessionTo', { name: node.requirement.defaultName ? t('navigation.general') : node.requirement.name })" :title="t('session.new')" :disabled="busy" @click="emit('newSession', node.requirement)"><Plus :size="14" /></button>
                </div>
                <button v-if="node.requirement.branchGroup" class="requirement-branch" :aria-label="t('branchGroup.view', { prefix: node.requirement.branchGroup.prefix })" :title="node.requirement.branchGroup.prefix" @click="emit('viewRequirement', node.requirement, 'gitBranch')"><GitBranch :size="12" /><span>{{ node.requirement.branchGroup.prefix }}/</span></button>
                <div :id="`requirement-${node.requirement.id}`" class="tree-panel" :class="{ expanded: expansion[`requirement:${node.requirement.id}`] }" :inert="!expansion[`requirement:${node.requirement.id}`]">
                  <div class="tree-panel-inner">
                    <ul class="tree-session-list">
                      <li v-for="session in node.sessions" :key="session.id">
                        <InlineName v-slot="{ rename }" :name="sessionTitle(session)" :label="t('navigation.renameSession', { name: sessionTitle(session) })" :disabled="busy" :save="name => props.rename('session', session.id, name)">
                          <RouterLink :to="`/sessions/${session.id}`" class="session-link" :class="{ selected: selectedId === session.id }" :title="`${sessionTitle(session)}\n${t('navigation.renameHint')}`" :aria-current="selectedId === session.id ? 'page' : undefined" aria-keyshortcuts="F2" @click="emit('navigate')" @dblclick.stop.prevent="rename" @keydown.f2.stop.prevent="rename">
                            <strong>{{ sessionTitle(session) }}</strong><small v-if="node.mixedAgents" class="session-agent">{{ endpoints.find(value => value.id === session.endpointId)?.name }}</small><span class="sr-only">{{ stateLabel(session.state) }}</span><i v-if="session.state !== 'idle'" class="state-dot" :class="session.state" aria-hidden="true" :title="stateLabel(session.state)"></i>
                          </RouterLink>
                        </InlineName>
                      </li>
                    </ul>
                  </div>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </li>
    </ul>
    <p v-if="!projects.length" class="empty-list">{{ t('navigation.empty') }}</p>
  </nav>
</template>
