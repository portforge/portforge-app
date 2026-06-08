<script setup>
import { ref, watch, computed } from 'vue'
import {
  GetRom, GetRomFilePaths, GetRomState,
  LaunchRom, GetDuckStationPath, SetDuckStationPath, SelectExecutable,
} from '../../wailsjs/go/main/App'
import { artworkAspectRatio, primaryArtwork, defaultArtworkType } from '../utils/artwork.js'

const props = defineProps({
  rom:    { type: Object, required: true }, // slim object from listing
})

const emit = defineEmits(['back'])

const enc = encodeURIComponent

// PortForge is opinionated about which emulator handles each platform — this
// mirrors the backend's emulatorLaunchCommand dispatch in app.go.
const PLATFORM_EMULATORS = {
  PS1Rom: { name: 'DuckStation', get: GetDuckStationPath, set: SetDuckStationPath },
}

const fullRom   = ref(null)
const filePaths = ref({})
const romState  = ref(null)

const emulatorPath = ref('')

const launching    = ref(false)
const launchError  = ref(null)
const showFormatMenu = ref(false)

const showSetupForm = ref(false)
const setupPath     = ref('')

async function loadDetail() {
  const [romResult, pathResult, stateResult] = await Promise.allSettled([
    GetRom(props.rom._itemTitle),
    GetRomFilePaths(props.rom._itemTitle),
    GetRomState(props.rom._itemTitle),
  ])
  fullRom.value   = romResult.status === 'fulfilled' ? romResult.value : props.rom
  filePaths.value = pathResult.status === 'fulfilled' ? pathResult.value ?? {} : {}
  romState.value  = stateResult.status === 'fulfilled' ? stateResult.value : null

  emulatorPath.value = ''
  const emulator = PLATFORM_EMULATORS[(fullRom.value ?? props.rom)._itemType]
  if (emulator) {
    emulatorPath.value = await emulator.get().catch(() => '')
  }
  setupPath.value      = emulatorPath.value
  showSetupForm.value  = false
  showFormatMenu.value = false
  launchError.value    = null
}

watch(() => props.rom, loadDetail, { immediate: true })

const emulator = computed(() => PLATFORM_EMULATORS[(fullRom.value ?? props.rom)._itemType] ?? null)

const presentFormats = computed(() => {
  const source = fullRom.value ?? props.rom
  return (source.formats ?? []).filter(isPresent)
})

// The format that Play launches by default: the last-launched one if it's still
// present, otherwise the first present format.
const primaryFormat = computed(() => {
  if (!presentFormats.value.length) return null
  const last = romState.value?.lastFormat
  return presentFormats.value.find(f => f.filename === last) ?? presentFormats.value[0]
})

const extraFormats = computed(() =>
  presentFormats.value.filter(f => f !== primaryFormat.value)
)

async function play(formatFilename = '') {
  showFormatMenu.value = false
  launchError.value = null
  launching.value = true
  try {
    await LaunchRom(props.rom._itemTitle, formatFilename)
  } catch (e) {
    launchError.value = String(e)
  } finally {
    launching.value = false
  }
}

async function browseSetupEmulator() {
  const chosen = await SelectExecutable().catch(() => null)
  if (chosen) {
    setupPath.value = chosen
    await saveSetup()
  }
}

async function saveSetup() {
  if (!emulator.value || !setupPath.value) return
  try {
    await emulator.value.set(setupPath.value)
    emulatorPath.value = setupPath.value
    showSetupForm.value = false
  } catch (e) {
    launchError.value = String(e)
  }
}

const coverArtwork = computed(() => {
  const source = fullRom.value ?? props.rom
  return primaryArtwork(source.artwork, source._itemType)
})
const coverAspectRatio = computed(() => {
  const source = fullRom.value ?? props.rom
  const type = coverArtwork.value?.artworkType ?? defaultArtworkType(source._itemType)
  return artworkAspectRatio(type)
})

function artworkUrl(type) {
  const source = fullRom.value ?? props.rom
  const art = type === 'cover'
    ? coverArtwork.value
    : source.artwork?.find(a => a.artworkType.toLowerCase() === type) ?? null
  if (!art) return null
  return `/mediaitems/${enc(source._itemType)}/${enc(source._itemTitle)}/.artwork/${enc(art.fileName)}`
}

function formatSize(bytes) {
  if (!bytes) return '—'
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
  if (bytes >= 1048576)    return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024)       return (bytes / 1024).toFixed(1) + ' KB'
  return bytes + ' B'
}

function isPresent(fmt) {
  return !!filePaths.value[fmt.checksums?.md5?.toLowerCase()]
}

function localPath(fmt) {
  return filePaths.value[fmt.checksums?.md5?.toLowerCase()] ?? null
}

function basename(path) {
  return path?.split(/[\\/]/).pop() ?? path
}
</script>

<template>
  <div class="detail">

    <div v-if="artworkUrl('banner')" class="banner-bg">
      <img :src="artworkUrl('banner')" :alt="fullRom?.title || rom._itemTitle" />
    </div>

    <div class="detail-content">
      <button class="back-btn" @click="emit('back')">&#8592; ROMs</button>

      <div class="detail-header">
        <img
          v-if="artworkUrl('cover')"
          :src="artworkUrl('cover')"
          :alt="fullRom?.title || rom._itemTitle"
          class="detail-art"
          :style="{ aspectRatio: coverAspectRatio }"
        />
        <div v-else class="detail-art-placeholder" :style="{ aspectRatio: coverAspectRatio }" />

        <div class="detail-info">
          <h1 class="detail-title">{{ fullRom?.title || rom._itemTitle }}</h1>
          <span v-if="fullRom?.platform" class="platform-tag">{{ fullRom.platform }}</span>

          <!-- Play area -->
          <div class="action-area">
            <p v-if="!presentFormats.length" class="action-notice">
              No ROM file in your library yet. Drop one onto this page to add it.
            </p>

            <template v-else-if="!emulator">
              <p class="action-notice">PortForge doesn't support an emulator for this platform yet.</p>
            </template>

            <template v-else-if="!emulatorPath">
              <p class="action-notice">{{ emulator.name }} isn't set up yet.</p>
              <button v-if="!showSetupForm" class="btn-ghost-sm" @click="showSetupForm = true">Set up {{ emulator.name }}</button>
            </template>

            <template v-else>
              <div class="btn-play-group">
                <button class="btn-play" :disabled="launching" @click="play(primaryFormat?.filename ?? '')">
                  {{ launching ? 'Launching…' : 'Play' }}
                </button>
                <button
                  v-if="extraFormats.length"
                  class="btn-play-arrow"
                  @click="showFormatMenu = !showFormatMenu"
                  title="More formats"
                >&#9660;</button>
                <div v-if="showFormatMenu" class="exe-menu">
                  <button
                    v-for="fmt in extraFormats"
                    :key="fmt.filename"
                    class="exe-menu-item"
                    @click="play(fmt.filename)"
                  >{{ fmt.format || fmt.ext }} — {{ basename(fmt.filename) }}</button>
                </div>
              </div>
              <button
                v-if="!showSetupForm"
                class="btn-ghost-sm"
                @click="showSetupForm = true"
              >Change {{ emulator.name }} location</button>
            </template>

            <p v-if="launchError" class="action-notice error">{{ launchError }}</p>

            <div v-if="showSetupForm" class="override-form">
              <div class="override-row">
                <input
                  class="override-input"
                  v-model="setupPath"
                  :placeholder="`Path to ${emulator?.name ?? 'emulator'} executable`"
                  spellcheck="false"
                />
                <button class="btn-ghost-sm" @click="browseSetupEmulator">Browse…</button>
              </div>
              <div class="override-row">
                <button class="btn-ghost-sm" @click="saveSetup">Save</button>
                <button class="btn-ghost-sm" @click="showSetupForm = false">Cancel</button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- format details -->
      <div v-if="fullRom?.formats?.length" class="section">
        <h2 class="section-heading">Formats</h2>
        <div class="format-list">
          <div v-for="fmt in fullRom.formats" :key="fmt.filename" class="format-card">

            <div class="format-header">
              <div class="format-identity">
                <span class="format-name-badge">{{ fmt.format || fmt.ext }}</span>
                <span class="format-filename">{{ fmt.filename }}</span>
              </div>
              <span class="format-size-label">{{ formatSize(fmt.filesize) }}</span>
            </div>

            <div class="format-presence" :class="isPresent(fmt) ? 'have' : 'missing'">
              <span v-if="isPresent(fmt)">
                ✓ Present — <code class="file-path">{{ basename(localPath(fmt)) }}</code>
              </span>
              <span v-else>✗ Not in library</span>
            </div>

            <div class="checksums">
              <div v-if="fmt.checksums?.md5"    class="checksum-row"><span class="ck-label">MD5</span>    <code>{{ fmt.checksums.md5 }}</code></div>
              <div v-if="fmt.checksums?.sha1"   class="checksum-row"><span class="ck-label">SHA-1</span>  <code>{{ fmt.checksums.sha1 }}</code></div>
              <div v-if="fmt.checksums?.sha256" class="checksum-row"><span class="ck-label">SHA-256</span><code>{{ fmt.checksums.sha256 }}</code></div>
              <div v-if="fmt.checksums?.crc32"  class="checksum-row"><span class="ck-label">CRC-32</span> <code>{{ fmt.checksums.crc32 }}</code></div>
            </div>

          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.detail {
  flex: 1;
  overflow-y: auto;
  position: relative;
}

.banner-bg {
  position: absolute;
  inset: 0;
  height: 340px;
  overflow: hidden;
  pointer-events: none;

  &::after {
    content: '';
    position: absolute;
    inset: 0;
    background: linear-gradient(to bottom, rgba(27,38,54,0.55) 0%, #303030 100%);
  }

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center top;
    opacity: 0.35;
  }
}

.detail-content {
  position: relative;
  padding: 24px 32px 48px;
  max-width: 900px;
  display: flex;
  flex-direction: column;
  gap: 32px;
}

.back-btn {
  background: none;
  border: none;
  color: #8b929a;
  font: inherit;
  font-size: 13px;
  padding: 0;
  cursor: pointer;
  align-self: flex-start;
  &:hover { color: #d6d6d6; }
}

/* ── Header ── */
.detail-header {
  display: flex;
  align-items: flex-start;
  gap: 28px;
}

.detail-art {
  max-height: 320px;
  width: auto;
  border-radius: 6px;
  flex-shrink: 0;
  box-shadow: 0 8px 32px rgba(0,0,0,0.5);
}

.detail-art-placeholder {
  width: 180px;
  border-radius: 6px;
  flex-shrink: 0;
  background: #2a2a2a;
  border: 1px solid #3e3e3e;
}

.detail-info {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 8px;
}

.detail-title {
  font-size: 26px;
  font-weight: 700;
  color: #e8eaed;
  margin: 0;
  line-height: 1.2;
}

.platform-tag {
  display: inline-block;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.6px;
  color: #8b929a;
  background: #3a3a3a;
  border: 1px solid #4e4e4e;
  border-radius: 4px;
  padding: 2px 8px;
  align-self: flex-start;
}

/* ── Play area ── */
.action-area {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-start;
}

.action-notice {
  font-size: 14px;
  color: #8b929a;
  margin: 0;

  &.error { color: #e06c75; }
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
  &:hover:not(:disabled) { background: #65d98a; }
  &:disabled { opacity: 0.6; cursor: default; }
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
  min-width: 220px;
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

.override-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-width: 420px;
}

.override-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.override-input {
  flex: 1;
  font: inherit;
  font-size: 13px;
  color: #b4b4b4;
  background: #323232;
  border: 1px solid #4e4e4e;
  border-radius: 6px;
  padding: 8px 12px;
  outline: none;
  min-width: 0;

  &::placeholder { color: #6e6e6e; }
}

.format-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}

.format-pill {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  border-radius: 4px;
  padding: 3px 10px;

  &.have    { background: rgba(80,200,120,0.15); color: #50c878; border: 1px solid rgba(80,200,120,0.35); }
  &.missing { background: rgba(224,108,117,0.12); color: #e06c75; border: 1px solid rgba(224,108,117,0.3); }
}

/* ── Sections ── */
.section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-heading {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.8px;
  color: #666;
  margin: 0;
}

/* ── Format cards ── */
.format-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.format-card {
  background: #2e2e2e;
  border: 1px solid #4e4e4e;
  border-radius: 8px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.format-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.format-identity {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.format-name-badge {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: #a0a0a0;
  background: rgba(255,255,255,0.07);
  border: 1px solid rgba(255,255,255,0.12);
  border-radius: 3px;
  padding: 2px 8px;
  flex-shrink: 0;
}

.format-filename {
  font-size: 13px;
  font-family: monospace;
  color: #8b929a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.format-size-label {
  font-size: 12px;
  color: #666;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}

.format-presence {
  font-size: 12px;
  &.have    { color: #50c878; }
  &.missing { color: #666; }
}

.file-path {
  font-family: monospace;
  font-size: 11px;
  color: #8b929a;
}

.checksums {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.checksum-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-size: 11px;

  code {
    font-family: monospace;
    color: #666;
    word-break: break-all;
  }
}

.ck-label {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #555;
  width: 52px;
  flex-shrink: 0;
}
</style>
