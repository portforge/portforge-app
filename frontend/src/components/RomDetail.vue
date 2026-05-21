<script setup>
import { ref, watch, computed } from 'vue'
import { GetRom, GetRomFilePaths } from '../../wailsjs/go/main/App'
import { artworkAspectRatio, primaryArtwork, defaultArtworkType } from '../utils/artwork.js'

const props = defineProps({
  rom:    { type: Object, required: true }, // slim object from listing
})

const emit = defineEmits(['back'])

const enc = encodeURIComponent

const fullRom   = ref(null)
const filePaths = ref({})

async function loadDetail() {
  const [romResult, pathResult] = await Promise.allSettled([
    GetRom(props.rom._itemTitle),
    GetRomFilePaths(props.rom._itemTitle),
  ])
  fullRom.value   = romResult.status === 'fulfilled' ? romResult.value : props.rom
  filePaths.value = pathResult.status === 'fulfilled' ? pathResult.value ?? {} : {}
}

watch(() => props.rom, loadDetail, { immediate: true })

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
