<script setup>
import { ref, onMounted } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  GetSettings, SaveSettings, SelectFolder, ValidateMediaItemsPath,
  GetDefaultPaths, GetMediaItemsSHA, CheckMediaItemsUpdate, SyncMediaItems,
} from '../../wailsjs/go/main/App'

const emit = defineEmits(['saved'])

const props = defineProps({
  /** If true, renders the full-screen first-run setup layout instead of the settings panel */
  setup: { type: Boolean, default: false },
})

const libraryPath = ref('')
const dataPath = ref('')
const saving = ref(false)
const error = ref(null)

const installedSHA = ref('')
const updateAvailable = ref(false)
const downloading = ref(false)
const downloadPhase = ref('')
const downloadPercent = ref(0)
const checkingUpdate = ref(false)

onMounted(async () => {
  const [settings, defaults] = await Promise.all([
    GetSettings().catch(() => null),
    GetDefaultPaths().catch(() => ({})),
  ])
  libraryPath.value = settings?.mediaItemsPath || defaults.mediaItemsPath || ''
  dataPath.value    = settings?.dataPath       || defaults.dataPath       || ''
  installedSHA.value = await GetMediaItemsSHA().catch(() => '')

  EventsOn('mediaitems:progress', ({ phase, percent }) => {
    downloadPhase.value   = phase
    downloadPercent.value = percent
    if (phase === 'done') downloading.value = false
  })
})

function beforeUnmount() {
  EventsOff('mediaitems:progress')
}

async function browseLibrary() {
  const chosen = await SelectFolder()
  if (chosen) { libraryPath.value = chosen; await save() }
}

async function browseData() {
  const chosen = await SelectFolder()
  if (chosen) { dataPath.value = chosen; await save() }
}

async function checkUpdate() {
  checkingUpdate.value = true
  error.value = null
  try {
    updateAvailable.value = await CheckMediaItemsUpdate()
  } catch (e) {
    error.value = String(e)
  } finally {
    checkingUpdate.value = false
  }
}

async function syncMediaItems() {
  if (!libraryPath.value) {
    error.value = 'Set the MediaItems library folder path first.'
    return
  }
  downloading.value = true
  downloadPhase.value = 'downloading'
  downloadPercent.value = 0
  error.value = null
  try {
    await SyncMediaItems(libraryPath.value)
    installedSHA.value = await GetMediaItemsSHA()
    updateAvailable.value = false
  } catch (e) {
    error.value = String(e)
    downloading.value = false
  }
}

async function save() {
  if (!libraryPath.value || !dataPath.value) return
  saving.value = true
  error.value = null
  try {
    await SaveSettings(libraryPath.value, dataPath.value)
    const warning = await ValidateMediaItemsPath()
    if (warning) {
      error.value = warning
      return
    }
    emit('saved')
  } catch (e) {
    error.value = String(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div :class="setup ? 'setup-screen' : 'settings-panel'">
    <div class="settings-content">
      <template v-if="setup">
        <h1 class="setup-title">Welcome to PortForge</h1>
        <p class="setup-subtitle">Confirm where your MediaItems library and user library are stored, then click Get Started.</p>
      </template>
      <template v-else>
        <h2 class="settings-heading">Settings</h2>
      </template>

      <!-- MediaItems library folder -->
      <div class="field-group">
        <label class="field-label">MediaItems library folder</label>
        <p class="field-hint">Read-only. Contains game metadata, artwork, and install specs.</p>
        <div class="path-row">
          <input class="path-input" v-model="libraryPath" placeholder="No folder selected" spellcheck="false" />
          <button class="btn-browse" @click="browseLibrary">Browse…</button>
        </div>

        <!-- Download / update controls -->
        <div v-if="downloading" class="mediaitems-status">
          <div class="progress-bar">
            <div class="progress-fill" :style="{ width: downloadPercent + '%' }" />
          </div>
          <span class="status-label">{{
            downloadPhase === 'extracting' ? 'Extracting…' :
            downloadPhase === 'copying'    ? 'Copying…' :
            `Downloading… ${downloadPercent}%`
          }}</span>
        </div>
        <div v-else class="mediaitems-controls">
          <span v-if="installedSHA === 'unknown'" class="status-label muted">Version unknown</span>
          <span v-else-if="installedSHA" class="sha-badge">{{ installedSHA }}</span>
          <span v-else class="status-label muted">Not synced</span>
          <button class="btn-action" @click="syncMediaItems">Sync</button>
          <template v-if="installedSHA && installedSHA !== 'unknown'">
            <button class="btn-action" :disabled="checkingUpdate" @click="checkUpdate">
              {{ checkingUpdate ? 'Checking…' : 'Check for updates' }}
            </button>
            <span v-if="updateAvailable === true" class="update-badge">Update available</span>
            <span v-else-if="updateAvailable === false && !checkingUpdate" class="status-label muted">Up to date</span>
          </template>
        </div>
      </div>

      <!-- User library folder -->
      <div class="field-group">
        <label class="field-label">User library folder</label>
        <p class="field-hint">Writable. Contains your ROM files, installed games, and save states.</p>
        <div class="path-row">
          <input class="path-input" v-model="dataPath" placeholder="No folder selected" spellcheck="false" />
          <button class="btn-browse" @click="browseData">Browse…</button>
        </div>
      </div>

      <p v-if="error" class="field-error">{{ error }}</p>

      <button
        class="btn-save"
        :disabled="!libraryPath || !dataPath || saving"
        @click="save"
      >{{ setup ? 'Get Started' : 'Save' }}</button>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.setup-screen {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.settings-panel {
  padding: 32px 24px;
  max-width: 640px;
}

.settings-content {
  display: flex;
  flex-direction: column;
  gap: 24px;
  max-width: 520px;
}

.setup-title {
  font-size: 26px;
  font-weight: 700;
  color: #e8eaed;
  margin: 0;
}

.setup-subtitle {
  font-size: 14px;
  color: #8b929a;
  margin: 0;
  line-height: 1.6;
}

.settings-heading {
  font-size: 18px;
  font-weight: 700;
  color: #e8eaed;
  margin: 0;
}

.field-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.8px;
  color: #8b929a;
}

.field-hint {
  font-size: 12px;
  color: #666666;
  margin: 0;
  line-height: 1.5;
}

.path-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #323232;
  border: 1px solid #4e4e4e;
  border-radius: 6px;
  padding: 8px 12px;
}

.path-input {
  flex: 1;
  font: inherit;
  font-size: 13px;
  color: #b4b4b4;
  background: none;
  border: none;
  outline: none;
  min-width: 0;

  &::placeholder { color: #8b929a; }
}

.btn-browse {
  flex-shrink: 0;
  background: none;
  border: 1px solid #4e4e4e;
  border-radius: 4px;
  color: #8b929a;
  font: inherit;
  font-size: 12px;
  padding: 3px 10px;
  cursor: pointer;

  &:hover {
    color: #b4b4b4;
    border-color: #646464;
  }
}

.mediaitems-controls {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.mediaitems-status {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.progress-bar {
  height: 4px;
  background: #4e4e4e;
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: #50c878;
  border-radius: 2px;
  transition: width 0.2s;
}

.sha-badge {
  font-family: monospace;
  font-size: 12px;
  color: #8b929a;
  background: #2a2a2a;
  border: 1px solid #4e4e4e;
  border-radius: 3px;
  padding: 2px 7px;
}

.update-badge {
  font-size: 12px;
  color: #50c878;
  background: rgba(80, 200, 120, 0.12);
  border: 1px solid rgba(80, 200, 120, 0.3);
  border-radius: 3px;
  padding: 2px 7px;
}

.status-label {
  font-size: 12px;
  color: #8b929a;

  &.muted { color: #666; }
}

.btn-action {
  background: none;
  border: 1px solid #4e4e4e;
  border-radius: 4px;
  color: #8b929a;
  font: inherit;
  font-size: 12px;
  padding: 3px 10px;
  cursor: pointer;

  &:hover:not(:disabled) {
    color: #b4b4b4;
    border-color: #646464;
  }
  &:disabled { opacity: 0.4; cursor: default; }
}

.field-error {
  font-size: 13px;
  color: #e06c75;
  margin: 0;
}

.btn-save {
  align-self: flex-start;
  background: #d4d4d4;
  color: #111111;
  border: none;
  border-radius: 5px;
  padding: 8px 22px;
  font: inherit;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;

  &:hover:not(:disabled) { background: #e8e8e8; }
  &:disabled { opacity: 0.4; cursor: default; }
}
</style>
