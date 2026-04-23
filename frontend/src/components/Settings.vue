<script setup>
import { ref } from 'vue'
import { GetSettings, SaveSettings, SelectFolder } from '../../wailsjs/go/main/App'

const emit = defineEmits(['saved'])

const props = defineProps({
  /** If true, renders the full-screen first-run setup layout instead of the settings panel */
  setup: { type: Boolean, default: false },
})

const libraryPath = ref('')
const dataPath = ref('')
const saving = ref(false)
const error = ref(null)

GetSettings().then(s => {
  libraryPath.value = s.mediaItemsPath ?? ''
  dataPath.value = s.dataPath ?? ''
}).catch(e => {
  error.value = String(e)
})

async function browseLibrary() {
  const chosen = await SelectFolder()
  if (chosen) libraryPath.value = chosen
}

async function browseData() {
  const chosen = await SelectFolder()
  if (chosen) dataPath.value = chosen
}

async function save() {
  if (!libraryPath.value || !dataPath.value) return
  saving.value = true
  error.value = null
  try {
    await SaveSettings(libraryPath.value, dataPath.value)
    emit('saved')
  } catch (e) {
    error.value = String(e)
  } finally {
    saving.value = false
  }
}

const canSave = () => !!libraryPath.value && !!dataPath.value
</script>

<template>
  <div :class="setup ? 'setup-screen' : 'settings-panel'">
    <div class="settings-content">
      <template v-if="setup">
        <h1 class="setup-title">Welcome to PortForge</h1>
        <p class="setup-subtitle">Choose where your MediaItems library and user data are stored to get started.</p>
      </template>
      <template v-else>
        <h2 class="settings-heading">Settings</h2>
      </template>

      <div class="field-group">
        <label class="field-label">MediaItems library folder</label>
        <p class="field-hint">Read-only. Contains game metadata, artwork, and install specs.</p>
        <div class="path-row">
          <span class="path-display" :class="{ placeholder: !libraryPath }">
            {{ libraryPath || 'No folder selected' }}
          </span>
          <button class="btn-browse" @click="browseLibrary">Browse…</button>
        </div>
      </div>

      <div class="field-group">
        <label class="field-label">User data folder</label>
        <p class="field-hint">Writable. Contains your ROM files, installed games, and save states.</p>
        <div class="path-row">
          <span class="path-display" :class="{ placeholder: !dataPath }">
            {{ dataPath || 'No folder selected' }}
          </span>
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

.path-display {
  flex: 1;
  font-size: 13px;
  color: #b4b4b4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;

  &.placeholder {
    color: #8b929a;
  }
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
