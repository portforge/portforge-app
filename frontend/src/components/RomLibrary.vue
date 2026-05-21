<script setup>
import { computed } from 'vue'
import { artworkAspectRatio, primaryArtwork, defaultArtworkType } from '../utils/artwork.js'

const props = defineProps({
  roms:     { type: Array,  required: true },
  status:   { type: Object, required: true },
  platform: { type: Object, default: null },  // GamingPlatform entry from RomPlatforms
})

const emit = defineEmits(['select', 'back'])

const enc = encodeURIComponent

const sortedRoms = computed(() =>
  props.roms
    .filter(r => props.status[r._itemTitle]
      && (!props.platform || r._itemType === props.platform.gamesItemType))
    .sort((a, b) => (a.title || a._itemTitle).localeCompare(b.title || b._itemTitle))
)

function coverUrl(rom) {
  const art = primaryArtwork(rom.artwork, rom._itemType)
  if (!art) return null
  return `/mediaitems/${enc(rom._itemType)}/${enc(rom._itemTitle)}/.artwork/${enc(art.fileName)}`
}

function cardAspectRatio(rom) {
  const art = primaryArtwork(rom.artwork, rom._itemType)
  const type = art?.artworkType ?? defaultArtworkType(rom._itemType)
  return artworkAspectRatio(type)
}

function formatSize(bytes) {
  if (!bytes) return '—'
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
  if (bytes >= 1048576)    return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024)       return (bytes / 1024).toFixed(1) + ' KB'
  return bytes + ' B'
}
</script>

<template>
  <div class="rom-library">
    <button v-if="platform" class="back-btn" @click="emit('back')">&#8592; Platforms</button>
    <p v-if="sortedRoms.length === 0" class="empty">No ROMs in your library yet. Drop ROM files onto this window to add them.</p>

    <ul v-else class="rom-list">
      <li v-for="rom in sortedRoms" :key="rom._itemTitle">
        <button class="rom-card" @click="emit('select', rom)">

          <!-- artwork or empty square placeholder -->
          <div class="card-art" :style="{ aspectRatio: cardAspectRatio(rom) }">
            <img
              v-if="coverUrl(rom)"
              :src="coverUrl(rom)"
              :alt="rom.title || rom._itemTitle"
            />
          </div>

          <div class="card-info">
            <span class="card-title">{{ rom.title || rom._itemTitle }}</span>
            <span v-if="rom.platform" class="card-platform">{{ rom.platform }}</span>
          </div>

        </button>
      </li>
    </ul>
  </div>
</template>

<style lang="scss" scoped>
.rom-library {
  padding: 24px;
  height: 100%;
}

.back-btn {
  background: none;
  border: none;
  color: #8b929a;
  font: inherit;
  font-size: 13px;
  padding: 0;
  margin-bottom: 16px;
  cursor: pointer;
  display: block;
  &:hover { color: #d6d6d6; }
}

.empty {
  color: #8b929a;
  text-align: center;
  margin-top: 80px;
}

.rom-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 8px;
  align-items: start;
}

/* ── Card ── */
.rom-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px;
  background: #32323280;
  border: 1px solid #4c4c4c;
  border-radius: 8px;
  cursor: pointer;
  text-align: left;
  font: inherit;
  color: inherit;
  width: 100%;
  transition: background-color 0.12s, border-color 0.12s;

  &:hover {
    background: #4d4d4d;
    border-color: #d6d6d6;
  }
}

/* ── Cover art / placeholder ── */
.card-art {
  width: 100%;
  min-height: 40px; /* fallback for unknown artwork types */
  flex-shrink: 0;
  border-radius: 4px;
  overflow: hidden;
  background: #2a2a2a;
  display: flex;
  align-items: center;
  justify-content: center;

  img {
    width: 100%;
    height: auto;
    display: block;
  }
}

/* ── Status badge ── */
.status-badge {
  position: absolute;
  top: 12px;
  right: 12px;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;

  &.have {
    background: rgba(80, 200, 120, 0.85);
    color: #0d1f14;
  }
  &.missing {
    background: rgba(224, 108, 117, 0.85);
    color: #2a090b;
  }
}

/* ── Info section ── */
.card-info {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
  color: #c8c8c8;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-platform {
  font-size: 11px;
  color: #8b929a;
}

.card-formats {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 2px;
}

.format-tag {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  color: #8b929a;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 3px;
  padding: 1px 6px;
}

.format-size {
  font-weight: 400;
  text-transform: none;
  color: #666;
}
</style>
