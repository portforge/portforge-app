<script setup>
import { computed } from 'vue'

const props = defineProps({
  roms: { type: Array, required: true },
  status: { type: Object, required: true },
})

const sortedRoms = computed(() =>
  [...props.roms].sort((a, b) => {
    const aHave = props.status[a._itemTitle] ? 0 : 1
    const bHave = props.status[b._itemTitle] ? 0 : 1
    return aHave - bHave
  })
)

const enc = encodeURIComponent

function romArtUrl(rom) {
  return `/mediaitems/VideoGameRom/${enc(rom._itemTitle)}/.artwork/`
}

function formatSize(bytes) {
  if (!bytes) return '—'
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return bytes + ' B'
}
</script>

<template>
  <div class="rom-library">
    <p v-if="sortedRoms.length === 0" class="empty">No ROMs found in the mediaitems directory.</p>

    <ul v-else class="rom-list">
      <li v-for="rom in sortedRoms" :key="rom._itemTitle" class="rom-card">
        <div
          class="rom-status-indicator"
          :class="status[rom._itemTitle] ? 'have' : 'missing'"
          :title="status[rom._itemTitle] ? 'ROM file present' : 'ROM file missing'"
        >
          {{ status[rom._itemTitle] ? '✓' : '✗' }}
        </div>

        <div class="rom-info">
          <span class="rom-title">{{ rom.title || rom._itemTitle }}</span>
          <span v-if="rom.platform" class="rom-platform">{{ rom.platform }}</span>
        </div>

        <div class="rom-formats">
          <span v-for="fmt in rom.formats" :key="fmt.filename" class="rom-format">
            <span class="format-name">{{ fmt.format || fmt.ext }}</span>
            <span class="format-size">{{ formatSize(fmt.filesize) }}</span>
          </span>
        </div>
      </li>
    </ul>
  </div>
</template>

<style lang="scss" scoped>
.rom-library {
  padding: 24px;
  max-width: 960px;
  margin: 0 auto;
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
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rom-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 16px;
  background: #343434;
  border: 1px solid #4e4e4e;
  border-radius: 6px;
}

.rom-status-indicator {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  flex-shrink: 0;

  &.have {
    background: rgba(80, 200, 120, 0.15);
    color: #50c878;
    border: 1px solid rgba(80, 200, 120, 0.4);
  }

  &.missing {
    background: rgba(224, 108, 117, 0.15);
    color: #e06c75;
    border: 1px solid rgba(224, 108, 117, 0.4);
  }
}

.rom-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.rom-title {
  font-size: 14px;
  font-weight: 600;
  color: #b4b4b4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.rom-platform {
  font-size: 12px;
  color: #8b929a;
}

.rom-formats {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.rom-format {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.format-name {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  color: #a0a0a0;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 3px;
  padding: 1px 6px;
}

.format-size {
  font-size: 11px;
  color: #8b929a;
  font-variant-numeric: tabular-nums;
}
</style>
