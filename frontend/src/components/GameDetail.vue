<script setup>
import { ref, computed, inject, watch } from 'vue'
import { marked } from 'marked'
import { GetROMStatus, GetInstallState, GetInstallPrompts, GetPlatformAvailable, LaunchVersion, CleanBuildDir, UninstallVersion } from '../../wailsjs/go/main/App'


const props = defineProps({
  game: { type: Object, required: true },
  platform: { type: String, required: true },
})

const emit = defineEmits(['back'])

// Install state is owned by App.vue and shared via provide/inject
const activeInstall = inject('activeInstall')
const startInstall  = inject('startInstall')
const clearInstall  = inject('clearInstall')
const cancelInstall = inject('cancelInstall')

const romStatus = ref({})
const installState = ref(null)
const platformAvailable = ref(false)
const installError = ref(null)
const showExeMenu = ref(false)
const confirmUninstall = ref(false)

const installPrompts = ref([])
const selectedArgs = ref({})

// Derived from shared activeInstall
const isInstalling = computed(() =>
  activeInstall.value?.itemTitle === props.game._itemTitle && !activeInstall.value?.failed
)
const buildFailed = computed(() =>
  activeInstall.value?.itemTitle === props.game._itemTitle ? activeInstall.value.failed : null
)
const progressLabel = computed(() => {
  if (!activeInstall.value) return 'Installing…'
  const { stepLabel, stepIndex, stepTotal, phase, percent } = activeInstall.value
  if (stepLabel) return `Step ${stepIndex + 1}/${stepTotal}: ${stepLabel}`
  if (phase === 'downloading') return `Downloading… ${percent}%`
  if (phase === 'extracting') return 'Extracting…'
  if (phase === 'copying_roms') return 'Copying ROMs…'
  return 'Installing…'
})
const progressPercent = computed(() => {
  if (!activeInstall.value) return 0
  const { phase, percent, stepIndex, stepTotal } = activeInstall.value
  if (phase === 'done') return 100
  if (stepTotal > 0) return Math.round((stepIndex + 1) / stepTotal * 100)
  if (phase === 'downloading') return Math.round(percent * 0.8)
  if (phase === 'extracting') return 85
  if (phase === 'copying_roms') return 95
  return 0
})

const descriptionHtml = computed(() =>
  marked.parse(props.game.description ?? '')
)

const romsReady = computed(() => {
  if (installPrompts.value.length > 0) {
    for (const prompt of installPrompts.value) {
      const hasRomOptions = prompt.options?.some(o => o.romTitle)
      if (!hasRomOptions) continue
      if (!prompt.options.some(o => o.romTitle && o.romsReady)) return false
    }
    return true
  }
  if (!props.game.romDependencies?.length) return true
  return props.game.romDependencies.every(rom => romFound(rom) === true)
})

const canInstall = computed(() => {
  if (!romsReady.value) return false
  for (const prompt of installPrompts.value) {
    const val = selectedArgs.value[prompt.name]
    if (!val) return false
    if (prompt.type === 'choice') {
      const opt = prompt.options.find(o => o.value === val)
      if (opt && !opt.romsReady) return false
    }
  }
  return true
})

const primaryExecutable = computed(() =>
  installState.value?.executables?.[0] ?? null
)

const extraExecutables = computed(() =>
  installState.value?.executables?.slice(1) ?? []
)

async function loadState() {
  const [romResult, stateResult, promptResult, availResult] = await Promise.allSettled([
    GetROMStatus(props.game._itemTitle),
    GetInstallState(props.game._itemTitle),
    GetInstallPrompts(props.game._itemTitle),
    GetPlatformAvailable(props.game._itemTitle),
  ])
  romStatus.value = romResult.status === 'fulfilled' ? romResult.value : {}
  installState.value = stateResult.status === 'fulfilled' ? stateResult.value : null
  installPrompts.value = (promptResult.status === 'fulfilled' ? promptResult.value : null) ?? []
  platformAvailable.value = availResult.status === 'fulfilled' ? availResult.value : false

  // Auto-select the only ROM-ready option for each choice prompt.
  const newArgs = {}
  for (const prompt of installPrompts.value) {
    if (prompt.type === 'choice') {
      const isRomArg = prompt.options.some(o => o.romTitle)
      if (isRomArg) {
        const ready = prompt.options.filter(o => o.romsReady)
        if (ready.length === 1) newArgs[prompt.name] = ready[0].value
      } else if (prompt.options.length > 0) {
        newArgs[prompt.name] = prompt.options[0].value
      }
    }
  }
  selectedArgs.value = newArgs
}

watch(() => props.game, loadState, { immediate: true })

async function install() {
  installError.value = null
  const ok = await startInstall(props.game._itemTitle, selectedArgs.value)
  if (ok) {
    installState.value = await GetInstallState(props.game._itemTitle)
  }
}

async function launch(executablePath = '') {
  showExeMenu.value = false
  try {
    await LaunchVersion(props.game._itemTitle, executablePath)
  } catch (e) {
    installError.value = String(e)
  }
}

async function cleanBuild() {
  try {
    await CleanBuildDir(props.game._itemTitle)
  } catch (e) {
    installError.value = String(e)
  } finally {
    clearInstall()
  }
}

function dismissBuildFailed() {
  clearInstall()
}

async function uninstall() {
  confirmUninstall.value = false
  try {
    await UninstallVersion(props.game._itemTitle)
    installState.value = await GetInstallState(props.game._itemTitle)
  } catch (e) {
    installError.value = String(e)
  }
}

// When a background install for this game finishes (activeInstall goes null),
// refresh the install state so the Play button appears automatically.
watch(activeInstall, (cur, prev) => {
  if (prev?.itemTitle === props.game._itemTitle && cur === null) {
    loadState()
  }
})


function romFound(rom) {
  if (!rom.formats?.length) return null
  const results = rom.formats.map(f => romStatus.value[f.checksums.md5])
  if (results.some(r => r === true)) return true
  if (results.some(r => r === undefined)) return null
  return false
}

function artworkUrl(type) {
  const art = props.game.artwork?.find(a => a.artworkType.toLowerCase() === type)
    ?? (type === 'cover' ? props.game.artwork?.[0] : null)
  if (!art) return null
  return `/mediaitems/VideoGameVersion/${encodeURIComponent(props.game._itemTitle)}/.artwork/${encodeURIComponent(art.fileName)}`
}
</script>

<template>
  <div class="detail">
    <div v-if="artworkUrl('banner')" class="banner-bg">
      <img :src="artworkUrl('banner')" :alt="game.title || game._itemTitle" />
    </div>

    <div class="detail-content">
      <button class="back-btn" @click="emit('back')">&#8592; Library</button>

      <div class="detail-header">
        <img
          v-if="artworkUrl('cover')"
          :src="artworkUrl('cover')"
          :alt="game.title || game._itemTitle"
          class="detail-art"
        />
        <div class="detail-info">
          <h1 class="detail-title">{{ game.title || game._itemTitle }}</h1>
          <span class="detail-year">{{ game.releaseYear }}</span>
          <div class="detail-meta">
            <span v-if="game.versionType" class="version-type-tag">{{ game.versionType }}</span>
            <span v-for="p in game.platforms" :key="p" class="platform-tag">{{ p }}</span>
          </div>
          <div v-if="game.tags?.length" class="detail-tags">
            <span v-for="tag in game.tags" :key="tag" class="tag">{{ tag }}</span>
          </div>

          <!-- Action area -->
          <div class="action-area">
            <!-- Build failed banner -->
            <div v-if="buildFailed" class="build-failed">
              <p class="build-failed-msg">
                <strong>Build failed</strong> at step <code>{{ buildFailed.step }}</code>:<br />
                {{ buildFailed.error }}
              </p>
              <p class="build-log-hint">Full output saved to <code>install.log</code> in the game's data folder.</p>
              <div class="build-failed-actions">
                <button class="btn-danger" @click="cleanBuild">Delete build folder</button>
                <button class="btn-ghost-sm" @click="dismissBuildFailed">Keep it</button>
              </div>
            </div>

            <!-- Split play button + uninstall -->
            <div v-else-if="installState?.installed" class="installed-actions">
            <div class="btn-play-group">
              <button
                class="btn-play"
                @click="launch(primaryExecutable?.path ?? '')"
              >{{ primaryExecutable?.title ?? 'Play' }}</button>
              <button
                v-if="extraExecutables.length"
                class="btn-play-arrow"
                @click="showExeMenu = !showExeMenu"
                title="More executables"
              >&#9660;</button>
              <div v-if="showExeMenu" class="exe-menu">
                <button
                  v-for="exe in extraExecutables"
                  :key="exe.path"
                  class="exe-menu-item"
                  @click="launch(exe.path)"
                >{{ exe.title || exe.path }}</button>
              </div>
            </div>
            <button v-if="!confirmUninstall" class="btn-uninstall" @click="confirmUninstall = true">Uninstall</button>
            <div v-else class="uninstall-confirm">
              <span class="uninstall-confirm-label">Remove all installed files?</span>
              <div class="uninstall-confirm-btns">
                <button class="btn-danger" @click="uninstall">Yes, uninstall</button>
                <button class="btn-ghost-sm" @click="confirmUninstall = false">Cancel</button>
              </div>
            </div>
            </div>

            <div v-else-if="isInstalling" class="install-progress">
              <div class="install-progress-header">
                <span class="install-progress-label">{{ progressLabel }}</span>
                <button class="btn-stop" @click="cancelInstall()">Stop</button>
              </div>
              <div class="progress-bar">
                <div class="progress-fill" :style="{ width: progressPercent + '%' }" />
              </div>
            </div>

            <p v-else-if="!platformAvailable" class="action-notice">
              This game is not available on {{ platform }}.
            </p>

            <template v-else-if="!romsReady">
              <p class="action-notice roms-missing">Required ROMs are missing. Drop them onto this page to add them.</p>
            </template>

            <template v-else>
              <!-- Spec install with user-selectable args (e.g. ROM region choice) -->
              <template v-if="installPrompts.length">
                <div class="install-prompts">
                  <div v-for="prompt in installPrompts" :key="prompt.name" class="prompt-group">
                    <span class="prompt-label">{{ prompt.label }}</span>
                    <div v-if="prompt.type === 'choice'" class="prompt-options">
                      <button
                        v-for="opt in prompt.options"
                        :key="opt.value"
                        class="prompt-option"
                        :class="{
                          selected: selectedArgs[prompt.name] === opt.value,
                          unavailable: !opt.romsReady,
                        }"
                        @click="opt.romsReady && (selectedArgs[prompt.name] = opt.value)"
                      >
                        <span v-if="opt.romTitle" class="prompt-rom-indicator">{{ opt.romsReady ? '✓' : '✗' }}</span>
                        {{ opt.label }}
                      </button>
                    </div>
                  </div>
                </div>
                <button class="btn-install" :disabled="!canInstall" @click="install()">Install</button>
              </template>

              <button v-else class="btn-install" @click="install()">Install</button>

              <p v-if="installError" class="action-error">{{ installError }}</p>
            </template>
          </div>
          <div class="meta-area">
            <div v-if="game.romDependencies?.length" class="section">
              <h2 class="section-heading">Required ROMs</h2>
              <ul class="rom-list">
                <li v-for="rom in game.romDependencies" :key="rom.title" class="rom-item">
                  <span
                    class="rom-status"
                    :class="{
                      have: romFound(rom) === true,
                      missing: romFound(rom) === false,
                      unknown: romFound(rom) === null,
                    }"
                    :title="romFound(rom) === true ? 'ROM found' : romFound(rom) === false ? 'ROM not found' : 'Checking…'"
                  >
                    {{ romFound(rom) === true ? '✓' : romFound(rom) === false ? '✗' : '?' }}
                  </span>
                  <span class="rom-title">{{ rom.title }}</span>
                  <span class="rom-size">{{ ((rom.formats?.[0]?.filesize ?? 0) / 1024 / 1024).toFixed(1) }} MB</span>
                </li>
              </ul>
            </div>

            <div v-if="game.mods?.length" class="section">
              <h2 class="section-heading">Available Mods</h2>
              <ul class="mod-list">
                <li v-for="mod in game.mods" :key="mod.title" class="mod-item">
                  <span class="mod-type-tag">{{ mod.modType }}</span>
                  <span class="mod-title">{{ mod.title }}</span>
                </li>
              </ul>
            </div>

          </div>
        </div>
      </div>

      <div v-if="game.description" class="description" v-html="descriptionHtml" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.detail {
  margin: 0 auto;
  position: relative;
  padding: 24px;
}

.banner-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 640px;
  z-index: 0;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center;
    opacity: 0.05;
  }

  &::after {
    content: '';
    position: absolute;
    inset: 0;
    background: linear-gradient(to bottom, transparent 30%, #222222 100%);
  }
}

.detail-content {
  position: relative;
  z-index: 1;
  max-width: 960px;
  margin: 0 auto;
}

.back-btn {
  background: none;
  border: none;
  color: #d8d8d8;
  font: inherit;
  font-size: 14px;
  cursor: pointer;
  padding: 0;
  margin-bottom: 20px;
  display: inline-block;
}

.back-btn:hover {
  color: #e8e8e8;
}

.detail-header {
  display: flex;
  gap: 24px;
  align-items: flex-start;
  margin-bottom: 24px;
}

.detail-art {
  width: 420px;
  flex-shrink: 0;
  border-radius: 6px;
  object-fit: cover;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
}

.detail-info {
  flex: 1;
}

.detail-title {
  font-size: 28px;
  font-weight: 700;
  color: #e8e8e8;
  margin: 0 0 4px;
}

.detail-year {
  font-size: 14px;
  color: #8b929a;
}

.detail-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.version-type-tag {
  font-size: 11px;
  background: rgba(255, 255, 255, 0.06);
  color: #a0a0a0;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 3px;
  padding: 2px 7px;
}

.platform-tag {
  font-size: 12px;
  background: #434343;
  color: #8b929a;
  border-radius: 4px;
  padding: 3px 8px;
}

.detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.tag {
  font-size: 12px;
  background: #434343;
  color: #8b929a;
  border-radius: 4px;
  padding: 3px 8px;
}

.action-area {
  margin-top: 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-start;
}

.meta-area {
  margin-top: 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-end;
}

.btn-play-group {
  position: relative;
  display: flex;
  align-items: stretch;
}

.btn-play {
  background: #50c878;
  color: #0d1a0f;
  border: none;
  border-radius: 6px 0 0 6px;
  padding: 10px 28px;
  font: inherit;
  font-size: 15px;
  font-weight: 700;
  cursor: pointer;

  &:only-child { border-radius: 6px; }
  &:hover { background: #65d98a; }
}

.btn-play-arrow {
  background: #3db865;
  color: #0d1a0f;
  border: none;
  border-left: 1px solid rgba(0,0,0,0.15);
  border-radius: 0 6px 6px 0;
  padding: 10px 12px;
  font: inherit;
  font-size: 11px;
  cursor: pointer;

  &:hover { background: #4acc73; }
}

.exe-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  background: #363636;
  border: 1px solid #434343;
  border-radius: 6px;
  overflow: hidden;
  z-index: 20;
  min-width: 160px;
  box-shadow: 0 4px 16px rgba(0,0,0,0.4);
}

.exe-menu-item {
  display: block;
  width: 100%;
  background: none;
  border: none;
  color: #b4b4b4;
  font: inherit;
  font-size: 13px;
  padding: 8px 14px;
  text-align: left;
  cursor: pointer;

  &:hover { background: #343434; color: #e8e8e8; }
}

.installed-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-start;
}

.btn-uninstall {
  background: none;
  color: #888888;
  border: 1px solid #565656;
  border-radius: 4px;
  padding: 5px 14px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;

  &:hover { color: #e06c75; border-color: rgba(224, 108, 117, 0.5); }
}

.uninstall-confirm {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.uninstall-confirm-label {
  font-size: 13px;
  color: #b4b4b4;
}

.uninstall-confirm-btns {
  display: flex;
  gap: 8px;
}

.build-failed {
  background: rgba(224, 108, 117, 0.08);
  border: 1px solid rgba(224, 108, 117, 0.3);
  border-radius: 6px;
  padding: 14px;
  max-width: 460px;
}

.build-failed-msg {
  font-size: 13px;
  color: #e06c75;
  margin: 0 0 12px;
  line-height: 1.5;

  code {
    font-family: monospace;
    background: rgba(224, 108, 117, 0.15);
    padding: 1px 4px;
    border-radius: 3px;
  }
}

.build-failed-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}

.btn-danger {
  background: #c0392b;
  color: #fff;
  border: none;
  border-radius: 4px;
  padding: 5px 14px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;

  &:hover { background: #d44637; }
}

.btn-ghost-sm {
  background: none;
  color: #888888;
  border: 1px solid #565656;
  border-radius: 4px;
  padding: 5px 12px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;

  &:hover { color: #c8c8c8; border-color: #565656; }
}

.build-log-hint {
  font-size: 12px;
  color: #666666;
  margin: 0 0 10px;

  code {
    font-family: monospace;
    background: rgba(255,255,255,0.05);
    padding: 1px 4px;
    border-radius: 3px;
  }
}

.btn-install {
  background: #50c878;
  color: #0d1a0f;
  border: none;
  border-radius: 6px;
  padding: 10px 28px;
  font: inherit;
  font-size: 15px;
  font-weight: 700;
  cursor: pointer;

  &:hover { background: #65d98a; }
  &:disabled { opacity: 0.35; cursor: default; }
}

.install-progress {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 320px;
}

.install-progress-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.install-progress-label {
  font-size: 13px;
  color: #8b929a;
}

.btn-stop {
  flex-shrink: 0;
  background: none;
  border: 1px solid #565656;
  border-radius: 4px;
  color: #888888;
  font: inherit;
  font-size: 12px;
  padding: 2px 10px;
  cursor: pointer;

  &:hover { color: #e06c75; border-color: #e06c75; }
}

.progress-bar {
  height: 6px;
  background: #434343;
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: #c8c8c8;
  border-radius: 3px;
  transition: width 0.2s ease;
}

.action-label {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #8b929a;
  margin: 0;
}

.build-variant-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.build-variant {
  display: flex;
  align-items: center;
  gap: 10px;
}

.install-prompts {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 4px;
}

.prompt-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.prompt-label {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #8b929a;
}

.prompt-options {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.prompt-option {
  display: flex;
  align-items: center;
  gap: 6px;
  background: #303030;
  border: 1px solid #4e4e4e;
  border-radius: 6px;
  color: #b4b4b4;
  font: inherit;
  font-size: 13px;
  padding: 6px 14px;
  cursor: pointer;
  transition: border-color 0.1s;

  &:hover:not(.unavailable) { border-color: #707070; }
  &.selected { border-color: #c0c0c0; background: rgba(255, 255, 255, 0.06); color: #e8e8e8; }
  &.unavailable { opacity: 0.5; cursor: default; }
}

.prompt-rom-indicator {
  font-size: 11px;
  font-weight: 700;

  .prompt-option:not(.unavailable) & { color: #50c878; }
  .prompt-option.unavailable & { color: #e06c75; }
}

.action-notice {
  font-size: 14px;
  color: #8b929a;
  margin: 0;

  &.roms-missing { color: #e06c75; }
}

.action-error {
  font-size: 13px;
  color: #e06c75;
  margin: 0;
}

.description {
  font-size: 14px;
  line-height: 1.7;
  color: #8b929a;
  margin-bottom: 32px;
}

.description :deep(p) { margin: 0 0 12px; }
.description :deep(p:last-child) { margin-bottom: 0; }
.description :deep(strong) { color: #c8c8c8; }
.description :deep(em) { color: #c8c8c8; font-style: italic; }

.section {
  margin-bottom: 32px;
  width: 100%;
}

.section-heading {
  font-size: 18px;
  font-weight: 700;
  color: #e8e8e8;
  margin: 0 0 16px;
  padding-bottom: 8px;
  border-bottom: 1px solid #434343;
}

.rom-list,
.mod-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rom-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}

.rom-status {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
  flex-shrink: 0;
}

.rom-status.unknown {
  background: #3b3b3b;
  color: #888888;
}

.rom-status.have {
  background: rgba(80, 200, 120, 0.15);
  color: #50c878;
  border: 1px solid rgba(80, 200, 120, 0.4);
}

.rom-status.missing {
  background: rgba(224, 108, 117, 0.15);
  color: #e06c75;
  border: 1px solid rgba(224, 108, 117, 0.4);
}

.rom-title {
  color: #b4b4b4;
  flex: 1;
}

.rom-size {
  color: #8b929a;
  font-size: 12px;
}

.mod-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #b4b4b4;
}

.mod-type-tag {
  font-size: 11px;
  background: rgba(255, 255, 255, 0.06);
  color: #a0a0a0;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 3px;
  padding: 1px 6px;
}

</style>
