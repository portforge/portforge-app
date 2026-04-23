<script setup>
import { ref, computed, provide, onMounted, onUnmounted } from 'vue'
import { GetVersions, GetPlatform, GetRoms, GetRomLibraryStatus, GetActiveInstall, InstallVersion, CancelInstall, GetSettings, ValidateMediaItemsPath, MatchDroppedROMs, ImportROMs } from '../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import GameLibrary from './components/GameLibrary.vue'
import GameDetail from './components/GameDetail.vue'
import RomLibrary from './components/RomLibrary.vue'
import Settings from './components/Settings.vue'

const versions = ref([])
const selectedGame = ref(null)
const platform = ref('')
const error = ref(null)
const isDragging = ref(false)
const activeTab = ref('library')
const needsSetup = ref(false)
const libraryWarning = ref(null)

const roms = ref([])
const romStatus = ref({})

const pendingDrop = ref(null)  // ROMDropSummary from MatchDroppedROMs
const dropError = ref(null)
const dropMatching = ref(false)

// ── Global install state ────────────────────────────────────────────────────
// Persists across navigation so background installs are tracked app-wide.
const activeInstall = ref(null) // { itemTitle, phase, percent, stepLabel, stepIndex, stepTotal, failed }

async function startInstall(itemTitle, args = {}) {
  activeInstall.value = {
    itemTitle, phase: 'downloading', percent: 0,
    stepLabel: null, stepIndex: 0, stepTotal: 0, failed: null,
  }
  try {
    await InstallVersion(itemTitle, args)
    activeInstall.value = null
    return true
  } catch {
    if (activeInstall.value && !activeInstall.value.failed) {
      activeInstall.value = { ...activeInstall.value, failed: { step: '', error: 'Installation failed' } }
    }
    return false
  }
}

function clearInstall() {
  activeInstall.value = null
}

provide('activeInstall', activeInstall)
provide('startInstall', startInstall)
provide('clearInstall', clearInstall)
provide('cancelInstall', CancelInstall)

// Show the banner only when user has navigated away from the installing game
const showInstallBanner = computed(() =>
  activeInstall.value !== null &&
  selectedGame.value?._itemTitle !== activeInstall.value.itemTitle
)

const installBannerLabel = computed(() => {
  if (!activeInstall.value) return ''
  const { phase, percent, stepLabel, stepIndex, stepTotal } = activeInstall.value
  if (stepLabel) return `Step ${stepIndex + 1}/${stepTotal}: ${stepLabel}`
  if (phase === 'downloading') return `Downloading… ${percent}%`
  if (phase === 'extracting') return 'Extracting…'
  if (phase === 'copying_roms') return 'Copying ROMs…'
  return 'Installing…'
})

const installBannerPercent = computed(() => {
  if (!activeInstall.value) return 0
  const { phase, percent, stepIndex, stepTotal } = activeInstall.value
  if (phase === 'done') return 100
  if (stepTotal > 0) return Math.round((stepIndex + 1) / stepTotal * 100)
  if (phase === 'downloading') return Math.round(percent * 0.8)
  if (phase === 'extracting') return 85
  if (phase === 'copying_roms') return 95
  return 0
})

const playingTitle = ref(null)   // _itemTitle of the currently running game
const playSeconds = ref(0)
let playTimer = null

const playingVersion = computed(() =>
  playingTitle.value ? versions.value.find(v => v._itemTitle === playingTitle.value) ?? null : null
)

function coverUrl(version) {
  const art = version?.artwork?.find(a => a.artworkType.toLowerCase() === 'cover') ?? version?.artwork?.[0]
  if (!art) return null
  return `/mediaitems/VideoGameVersion/${encodeURIComponent(version._itemTitle)}/.artwork/${encodeURIComponent(art.fileName)}`
}

function formatPlaytime(secs) {
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  const s = secs % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

async function loadLibrary() {
  libraryWarning.value = await ValidateMediaItemsPath()
  try {
    [versions.value, platform.value] = await Promise.all([GetVersions(), GetPlatform()])
  } catch (e) {
    error.value = String(e)
  }
  try {
    [roms.value, romStatus.value] = await Promise.all([GetRoms(), GetRomLibraryStatus()])
  } catch { /* non-fatal */ }
}

async function onSettingsSaved() {
  needsSetup.value = false
  error.value = null
  activeTab.value = 'library'
  await loadLibrary()
}

onMounted(async () => {
  const settings = await GetSettings()
  if (!settings.mediaItemsPath || !settings.dataPath) {
    needsSetup.value = true
    return
  }
  await loadLibrary()

  // Recover any install that was already running (e.g. after a dev hot-reload)
  const recovering = await GetActiveInstall()
  if (recovering && !activeInstall.value) {
    activeInstall.value = { itemTitle: recovering, phase: 'building', percent: 0, stepLabel: null, stepIndex: 0, stepTotal: 0, failed: null }
  }

  EventsOn('install:started', ({ itemTitle }) => {
    if (!activeInstall.value) {
      activeInstall.value = { itemTitle, phase: 'downloading', percent: 0, stepLabel: null, stepIndex: 0, stepTotal: 0, failed: null }
    }
  })
  EventsOn('install:progress', data => {
    if (activeInstall.value) activeInstall.value = { ...activeInstall.value, ...data }
  })
  EventsOn('install:step', ({ index, total, label }) => {
    if (activeInstall.value) activeInstall.value = { ...activeInstall.value, stepIndex: index, stepTotal: total, stepLabel: label }
  })
  EventsOn('install:failed', data => {
    if (activeInstall.value) activeInstall.value = { ...activeInstall.value, failed: data }
  })
  EventsOn('install:cancelled', () => {
    activeInstall.value = null
  })

  EventsOn('wails:file-drop', handleFileDrop)

  EventsOn('game:started', ({ itemTitle }) => {
    playingTitle.value = itemTitle
    playSeconds.value = 0
    playTimer = setInterval(() => { playSeconds.value++ }, 1000)
  })

  EventsOn('game:ended', () => {
    clearInterval(playTimer)
    playTimer = null
    playingTitle.value = null
  })
})

onUnmounted(() => {
  EventsOff('install:started')
  EventsOff('install:progress')
  EventsOff('install:step')
  EventsOff('install:failed')
  EventsOff('install:cancelled')
  EventsOff('wails:file-drop')
  EventsOff('game:started')
  EventsOff('game:ended')
  clearInterval(playTimer)
})

async function handleFileDrop(x, y, paths) {
  isDragging.value = false
  if (!paths?.length) return
  dropError.value = null
  pendingDrop.value = null
  dropMatching.value = true
  try {
    const summary = await MatchDroppedROMs(paths)
    if (summary.matched?.length) {
      pendingDrop.value = summary
    } else {
      dropError.value = `No files matched any known ROM in the library.`
    }
    if (summary.unmatched?.length && summary.matched?.length) {
      dropError.value = `${summary.unmatched.length} file(s) not recognised: ${summary.unmatched.join(', ')}`
    }
  } catch (e) {
    dropError.value = String(e)
  } finally {
    dropMatching.value = false
  }
}

async function confirmDrop(move) {
  if (!pendingDrop.value) return
  try {
    await ImportROMs(pendingDrop.value.matched, move)
    // Refresh ROM status
    try { romStatus.value = await GetRomLibraryStatus() } catch {}
    if (selectedGame.value) {
      selectedGame.value = { ...selectedGame.value, _romRefresh: Date.now() }
    }
  } catch (e) {
    dropError.value = String(e)
  } finally {
    pendingDrop.value = null
  }
}

function dismissDrop() {
  pendingDrop.value = null
  dropError.value = null
}
</script>

<template>
  <div
    id="shell"
    @dragenter.prevent="isDragging = true"
    @dragover.prevent="isDragging = true"
    @dragleave="isDragging = false"
    @drop.prevent="isDragging = false"
    :class="{ dragging: isDragging }"
  >
    <header>
      <span class="app-title">PortForge</span>
      <nav v-if="!selectedGame && !needsSetup" class="tab-nav">
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'library' }"
          @click="activeTab = 'library'"
        >Library</button>
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'roms' }"
          @click="activeTab = 'roms'"
        >ROMs</button>
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'settings' }"
          @click="activeTab = 'settings'"
        >Settings</button>
      </nav>
    </header>

    <div class="content-area">
      <div v-if="isDragging" class="drop-overlay">
        <div class="drop-overlay-inner">Drop ROM file here</div>
      </div>

      <div v-if="libraryWarning" class="library-warning-banner">
        <span>{{ libraryWarning }}</span>
        <button class="btn-ghost" @click="activeTab = 'settings'; selectedGame = null">Open Settings</button>
        <button class="btn-ghost" @click="libraryWarning = null">✕</button>
      </div>

      <div v-if="dropMatching" class="drop-banner">
        <span>Matching files…</span>
      </div>
      <div v-else-if="pendingDrop" class="drop-banner">
        <div class="drop-banner-info">
          <span class="drop-banner-lead">{{ pendingDrop.matched.length }} ROM{{ pendingDrop.matched.length !== 1 ? 's' : '' }} matched</span>
          <span class="drop-banner-titles">{{ pendingDrop.matched.map(m => m.romTitle).join(', ') }}</span>
        </div>
        <div class="drop-banner-actions">
          <button class="btn-primary" @click="confirmDrop(false)">Copy to library</button>
          <button class="btn-primary" @click="confirmDrop(true)">Move to library</button>
          <button class="btn-ghost" @click="dismissDrop">Dismiss</button>
        </div>
      </div>
      <div v-if="dropError" class="drop-error-banner">
        {{ dropError }}
        <button class="btn-ghost" @click="dropError = null">✕</button>
      </div>

      <div v-if="showInstallBanner" class="install-banner" @click.self="selectedGame = versions.find(v => v._itemTitle === activeInstall.itemTitle) ?? null">
        <div class="install-banner-text" style="cursor:pointer" @click="selectedGame = versions.find(v => v._itemTitle === activeInstall.itemTitle) ?? null">
          <span class="install-banner-title">Installing {{ activeInstall.failed ? '— failed' : '' }}</span>
          <span class="install-banner-label">{{ activeInstall.failed ? activeInstall.failed.error : installBannerLabel }}</span>
        </div>
        <div class="install-banner-bar">
          <div class="install-banner-fill" :style="{ width: installBannerPercent + '%' }" />
        </div>
        <button v-if="!activeInstall.failed" class="btn-stop-install" @click.stop="CancelInstall()">Stop</button>
      </div>

      <div v-if="playingVersion" class="now-playing-overlay">
        <div class="now-playing-card">
          <img
            v-if="coverUrl(playingVersion)"
            :src="coverUrl(playingVersion)"
            :alt="playingVersion.title || playingVersion._itemTitle"
            class="now-playing-cover"
          />
          <div class="now-playing-info">
            <span class="now-playing-label">Now Playing</span>
            <span class="now-playing-title">{{ playingVersion.title || playingVersion._itemTitle }}</span>
            <span class="now-playing-timer">{{ formatPlaytime(playSeconds) }}</span>
          </div>
        </div>
      </div>

      <main>
        <Settings v-if="needsSetup" :setup="true" @saved="onSettingsSaved" />
        <template v-else>
          <div v-if="error" class="error">{{ error }}</div>
          <GameDetail
            v-else-if="selectedGame"
            :game="selectedGame"
            :platform="platform"
            @back="selectedGame = null"
          />
          <Settings
            v-else-if="activeTab === 'settings'"
            @saved="onSettingsSaved"
          />
          <RomLibrary
            v-else-if="activeTab === 'roms'"
            :roms="roms"
            :status="romStatus"
          />
          <GameLibrary
            v-else
            :versions="versions"
            @select="selectedGame = $event"
          />
        </template>
      </main>
    </div>
  </div>
</template>

<style>
*, *::before, *::after {
  box-sizing: border-box;
}

html, body {
  margin: 0;
  padding: 0;
  height: 100%;
  background-color: #303030;
  color: #b4b4b4;
  font-family: "Nunito", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

@font-face {
  font-family: "Nunito";
  font-style: normal;
  font-weight: 400;
  src: local(""), url("assets/fonts/nunito-v16-latin-regular.woff2") format("woff2");
}

#app {
  height: 100vh;
}

#shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  position: relative;
}

#shell.dragging .content-area {
  outline: 2px dashed #d6d6d6;
  outline-offset: -4px;
}

header {
  display: flex;
  align-items: center;
  padding: 0 24px;
  height: 52px;
  background-color: #272727;
  border-bottom: 1px solid #1b1b1b;
  flex-shrink: 0;
}

.app-title {
  font-size: 18px;
  font-weight: 700;
  color: #e8eaed;
  letter-spacing: 0.5px;
}

.tab-nav {
  display: flex;
  gap: 4px;
  margin-left: 32px;
}

.tab-btn {
  background: none;
  border: none;
  color: #8b929a;
  font: inherit;
  font-size: 14px;
  cursor: pointer;
  padding: 4px 12px;
  border-radius: 4px;
  transition: color 0.1s;

  &:hover { color: #c6d4df; }
  &.active {
    color: #e8eaed;
    font-weight: 600;
    background: rgba(255, 255, 255, 0.06);
  }
}

/* ── Content area ── */
.content-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

main {
  flex: 1;
  overflow-y: auto;
}

/* ── Drop overlay ── */
.drop-overlay {
  position: absolute;
  inset: 0;
  background: rgba(255, 255, 255, 0.04);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  pointer-events: none;
}

.drop-overlay-inner {
  font-size: 20px;
  font-weight: 700;
  color: #d4d4d4;
  border: 2px dashed #555555;
  border-radius: 12px;
  padding: 32px 64px;
}

/* ── Banners ── */
.library-warning-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 24px;
  background: rgba(224, 176, 68, 0.08);
  border-bottom: 1px solid rgba(224, 176, 68, 0.25);
  color: #e0b044;
  font-size: 13px;
  flex-shrink: 0;

  span { flex: 1; }
}

.drop-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 24px;
  background: #353535;
  border-bottom: 1px solid #4e4e4e;
  font-size: 14px;
  flex-shrink: 0;
}

.drop-banner-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.drop-banner-lead {
  font-weight: 600;
  color: #c6d4df;
}

.drop-banner-titles {
  font-size: 12px;
  color: #8b929a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.drop-banner-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.drop-error-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 24px;
  background: rgba(224, 108, 117, 0.1);
  border-bottom: 1px solid rgba(224, 108, 117, 0.3);
  color: #e06c75;
  font-size: 14px;
  flex-shrink: 0;
}

.drop-error-banner button {
  margin-left: auto;
}

/* ── Install banner ── */
.install-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 24px;
  background: #353535;
  border-bottom: 1px solid #4e4e4e;
  font-size: 13px;
  flex-shrink: 0;
  cursor: pointer;

  &:hover { background: #404040; }
}

.install-banner-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.install-banner-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.8px;
  color: #c8c8c8;
}

.install-banner-label {
  font-size: 12px;
  color: #8b929a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.install-banner-bar {
  flex: 1;
  height: 4px;
  background: #434343;
  border-radius: 2px;
  overflow: hidden;
}

.install-banner-fill {
  height: 100%;
  background: #c8c8c8;
  border-radius: 2px;
  transition: width 0.3s ease;
}

.btn-stop-install {
  flex-shrink: 0;
  background: none;
  border: 1px solid #4a4a4a;
  border-radius: 4px;
  color: #888888;
  font: inherit;
  font-size: 12px;
  padding: 3px 10px;
  cursor: pointer;

  &:hover { color: #e06c75; border-color: #e06c75; }
}

/* ── Buttons ── */
.btn-primary {
  background: #d4d4d4;
  color: #111111;
  border: none;
  border-radius: 4px;
  padding: 4px 14px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}

.btn-primary:hover {
  background: #e8e8e8;
}

.btn-ghost {
  background: none;
  color: #888888;
  border: 1px solid #4e4e4e;
  border-radius: 4px;
  padding: 4px 12px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}

.btn-ghost:hover {
  color: #c8c8c8;
  border-color: #5e5e5e;
}

/* ── Now Playing overlay ── */
.now-playing-overlay {
  position: absolute;
  inset: 0;
  background: rgba(10, 10, 10, 0.92);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.now-playing-card {
  display: flex;
  gap: 32px;
  align-items: center;
}

.now-playing-cover {
  width: 200px;
  border-radius: 8px;
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.6);
}

.now-playing-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.now-playing-label {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  color: #50c878;
}

.now-playing-title {
  font-size: 32px;
  font-weight: 700;
  color: #e8eaed;
}

.now-playing-timer {
  font-size: 18px;
  color: #8b929a;
  font-variant-numeric: tabular-nums;
}

.error {
  color: #e06c75;
  padding: 16px;
  background: rgba(224, 108, 117, 0.1);
  border-radius: 6px;
  border: 1px solid rgba(224, 108, 117, 0.3);
}
</style>
