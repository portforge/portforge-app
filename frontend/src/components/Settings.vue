<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  GetSettings, ValidateMediaItemsPath,
  GetCatalogInfo, CheckMediaItemsUpdate, SyncMediaItems,
  GetLibraryStorage, RefreshLibraryIndex,
  GetStorageUnits, AddStorageUnit, RemoveStorageUnit, ReorderStorageUnits, OpenStorageUnit,
} from '../../wailsjs/go/main/App'

const emit = defineEmits(['saved', 'refreshed'])

const props = defineProps({
  /** If true, renders the full-screen first-run setup layout instead of the settings page */
  setup: { type: Boolean, default: false },
})

const saving = ref(false)
const error = ref(null)

const catalog = ref({ sha: '', syncedAt: '', portCount: 0, devMode: false })
const storage = ref({ available: false, totalBytes: 0, freeBytes: 0, libraryBytes: 0 })

// ── Storage units ───────────────────────────────────────────────────────────
// The ordered list of folders finished output is written to, shared with every
// other program in the MediaItem suite. Order is priority: the topmost folder
// with room receives new output.
const units = ref([])
const unitsError = ref(null)
const unitsBusy = ref(false)

async function loadUnits() {
  try {
    units.value = await GetStorageUnits()
    unitsError.value = null
  } catch (e) {
    units.value = []
    unitsError.value = String(e)
  }
}

async function addUnit() {
  unitsBusy.value = true
  try {
    const added = await AddStorageUnit()
    if (added) await loadUnits()
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  } finally {
    unitsBusy.value = false
  }
}

// Removing forgets the location; nothing stored there is touched. Worth saying
// out loud, because "remove" next to a folder full of games reads as "delete".
async function removeUnit(u) {
  unitsBusy.value = true
  try {
    await RemoveStorageUnit(u.id)
    await loadUnits()
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  } finally {
    unitsBusy.value = false
  }
}

// Reorder sends every id, so a stale list can't drop or invent a unit.
async function moveUnit(index, delta) {
  const next = index + delta
  if (next < 0 || next >= units.value.length) return
  const ids = units.value.map(u => u.id)
  ;[ids[index], ids[next]] = [ids[next], ids[index]]
  unitsBusy.value = true
  try {
    await ReorderStorageUnits(ids)
    await loadUnits()
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  } finally {
    unitsBusy.value = false
  }
}

async function openUnit(u) {
  try {
    await OpenStorageUnit(u.id)
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  }
}

function unitSpace(u) {
  if (u.unreachable) return 'Not connected'
  if (!u.totalBytes) return 'Size unavailable'
  return `${formatBytes(u.freeBytes)} free of ${formatBytes(u.totalBytes)}`
}

const refreshing = ref(false)
const updateAvailable = ref(null)   // null until checked
const checkingUpdate = ref(false)
const downloading = ref(false)
const downloadPhase = ref('')
const downloadPercent = ref(0)

onMounted(async () => {
  await Promise.all([loadCatalog(), loadStorage(), loadUnits()])

  EventsOn('mediaitems:progress', ({ phase, percent }) => {
    downloadPhase.value = phase
    downloadPercent.value = percent
    if (phase === 'done') downloading.value = false
  })
})

// The previous version declared this but never registered it, so every visit to
// Settings added another listener to the same event.
onBeforeUnmount(() => EventsOff('mediaitems:progress'))

async function loadCatalog() {
  catalog.value = await GetCatalogInfo().catch(() => catalog.value)
}

// Walks the library to size it, so it is only called on mount and after a sync.
async function loadStorage() {
  storage.value = await GetLibraryStorage().catch(() => storage.value)
}

// ── Formatting ──────────────────────────────────────────────────────────────
const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

function formatBytes(n) {
  if (!n || n < 0) return '0 B'
  let v = n, i = 0
  while (v >= 1024 && i < UNITS.length - 1) { v /= 1024; i++ }
  return `${i === 0 || v >= 100 ? Math.round(v) : v.toFixed(1)} ${UNITS[i]}`
}

function formatDate(iso) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, {
    day: 'numeric', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

// ── Catalog ─────────────────────────────────────────────────────────────────
const portCountLabel = computed(() => {
  const n = catalog.value.portCount
  return `${n} port${n === 1 ? '' : 's'} available`
})

const catalogLine = computed(() => {
  const { devMode, sha, syncedAt } = catalog.value
  if (devMode) return `Project-local catalog (dev mode) · ${portCountLabel.value}`
  if (!sha) return 'Not synced yet'
  if (sha === 'unknown') return `Present, but not synced by PortForge · ${portCountLabel.value}`
  const when = syncedAt ? formatDate(syncedAt) : ''
  return when
    ? `Last synced ${when} · ${portCountLabel.value}`
    : portCountLabel.value
})

const syncLabel = computed(() => {
  if (!downloading.value) return 'Refresh catalog'
  if (downloadPhase.value === 'extracting') return 'Extracting…'
  if (downloadPhase.value === 'copying') return 'Copying…'
  return `Downloading… ${downloadPercent.value}%`
})

async function syncCatalog() {
  downloading.value = true
  downloadPhase.value = 'downloading'
  downloadPercent.value = 0
  error.value = null
  try {
    await SyncMediaItems()
    updateAvailable.value = false
    await Promise.all([loadCatalog(), loadStorage(), loadUnits()])
    emit('refreshed')
  } catch (e) {
    error.value = String(e)
  } finally {
    downloading.value = false
  }
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

async function rebuildIndex() {
  refreshing.value = true
  error.value = null
  try {
    await RefreshLibraryIndex()
    emit('refreshed')
  } catch (e) {
    error.value = String(e)
  } finally {
    refreshing.value = false
  }
}

// save finishes first-run setup. There is no path to persist any more — the
// storage location was already written to the shared list when it was added — so
// this only confirms the catalog is usable before leaving the setup screen.
async function save() {
  if (!units.value.length) return
  saving.value = true
  error.value = null
  try {
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
  <!-- ── First run ───────────────────────────────────────────────────────── -->
  <div v-if="props.setup" class="setup-screen">
    <div class="setup-content">
      <h1 class="setup-title">Welcome to PortForge</h1>
      <p class="setup-subtitle">
        Choose where PortForge should keep what it builds and imports. This folder is
        shared with the other MediaItem programs on this machine, and you can add more
        later. You can sync the port catalog from Settings afterwards.
      </p>

      <div class="setup-units">
        <div v-for="u in units" :key="u.id" class="setup-unit">
          <span class="setup-unit-name">{{ u.name }}</span>
          <span class="setup-unit-path mono selectable">{{ u.path }}</span>
        </div>
        <button class="btn-outline" :disabled="unitsBusy" @click="addUnit">
          {{ units.length ? 'Add another folder…' : 'Choose a folder…' }}
        </button>
      </div>

      <p v-if="unitsError" class="error-text">{{ unitsError }}</p>
      <p v-if="error" class="error-text">{{ error }}</p>

      <button class="btn-primary" :disabled="!units.length || saving" @click="save">
        Get Started
      </button>
    </div>
  </div>

  <!-- ── Settings ────────────────────────────────────────────────────────── -->
  <div v-else class="settings">
    <h1 class="page-title settings-title">Settings</h1>

    <section class="card catalog-card">
      <div class="card-text">
        <span class="card-name">Port catalog</span>
        <span class="card-meta">{{ catalogLine }}</span>
      </div>

      <div class="card-actions">
        <span v-if="catalog.sha && catalog.sha !== 'unknown'" class="sha mono">{{ catalog.sha }}</span>
        <span v-if="updateAvailable === true" class="badge-update">Update available</span>
        <span v-else-if="updateAvailable === false && !checkingUpdate" class="badge-quiet">Up to date</span>

        <button
          v-if="!catalog.devMode && catalog.sha && catalog.sha !== 'unknown'"
          class="btn-outline"
          :disabled="checkingUpdate || downloading"
          @click="checkUpdate"
        >{{ checkingUpdate ? 'Checking…' : 'Check for updates' }}</button>

        <button
          class="btn-primary"
          :disabled="downloading || catalog.devMode"
          :title="catalog.devMode ? 'Syncing is disabled in dev mode' : undefined"
          @click="syncCatalog"
        >{{ syncLabel }}</button>
      </div>

      <div v-if="downloading" class="card-progress">
        <div class="card-progress-fill" :style="{ width: downloadPercent + '%' }" />
      </div>
    </section>

    <header class="section-head">
      <span class="section-name">Storage locations</span>
      <span class="spacer" />
      <button class="btn-outline" :disabled="unitsBusy" @click="addUnit">Add folder…</button>
    </header>
    <p class="section-desc">
      Shared with the other MediaItem programs on this machine, so a folder added here
      is a folder they all know about. Order is priority — new output goes to the
      topmost folder with room. Removing one forgets the location; nothing stored
      there is touched.
    </p>

    <section class="card unit-list">
      <div v-if="!units.length" class="unit-empty">
        <p>No storage locations yet.</p>
        <p class="unit-empty-hint">
          Choose where finished builds and imported files should be kept. Nothing is
          added for you — the folders here are only ever ones you pick.
        </p>
      </div>
      <div v-for="(u, i) in units" :key="u.id" class="unit" :class="{ offline: u.unreachable }">
        <div class="unit-rank mono">{{ i + 1 }}</div>
        <div class="unit-body">
          <div class="unit-name">
            {{ u.name }}
            <span v-if="u.unreachable" class="unit-badge">not connected</span>
          </div>
          <div class="unit-path mono selectable">{{ u.path }}</div>
        </div>
        <div class="unit-space">{{ unitSpace(u) }}</div>
        <div class="unit-actions">
          <button
            class="btn-icon"
            title="Move up"
            :disabled="i === 0 || unitsBusy"
            @click="moveUnit(i, -1)"
          >&uarr;</button>
          <button
            class="btn-icon"
            title="Move down"
            :disabled="i === units.length - 1 || unitsBusy"
            @click="moveUnit(i, 1)"
          >&darr;</button>
          <button
            class="btn-outline btn-small"
            :disabled="u.unreachable"
            :title="u.unreachable ? 'This folder isn\'t connected right now' : 'Show this folder in your file manager'"
            @click="openUnit(u)"
          >Open folder</button>
          <button class="btn-outline btn-small btn-danger" :disabled="unitsBusy" @click="removeUnit(u)">
            Remove
          </button>
        </div>
      </div>

      <div v-if="units.length && storage.available" class="unit-usage">
        <span class="legend-item">
          <i class="swatch swatch-library" />PortForge · {{ formatBytes(storage.libraryBytes) }}
        </span>
        <span class="unit-usage-in">in {{ units[0].name }}</span>
      </div>
    </section>

    <p v-if="unitsError" class="error-text">{{ unitsError }}</p>

    <header class="section-head">
      <span class="section-name">Maintenance</span>
    </header>
    <p class="section-desc">
      Rebuilds the search index from the catalog already on disk. Needed only after
      editing catalog files by hand — a sync does it for you.
    </p>

    <section class="card maintenance-card">
      <div class="card-text">
        <span class="card-name">Library index</span>
        <span class="card-meta">Rescans installed ports and matched ROMs.</span>
      </div>
      <div class="card-actions">
        <button class="btn-outline" :disabled="refreshing" @click="rebuildIndex">
          {{ refreshing ? 'Rebuilding…' : 'Rebuild index' }}
        </button>
      </div>
    </section>

    <p v-if="error" class="error-text">{{ error }}</p>
  </div>
</template>

<style lang="scss" scoped>
/* ── Page ─────────────────────────────────────────────────────────────────── */
.settings {
  max-width: 780px;
  padding: 18px var(--pad-page) 44px;
}

.settings-title {
  margin-bottom: 26px;
}

.section-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 26px;
}

.section-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
}

.spacer {
  flex: 1;
}

.section-desc {
  max-width: 60ch;
  margin: 6px 0 12px;
  font-size: 12.5px;
  color: var(--dim2);
}

/* ── Cards ────────────────────────────────────────────────────────────────── */
.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  padding: 18px 20px;
}

.catalog-card,
.maintenance-card {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.card-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.card-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
}

.card-meta {
  font-size: 12.5px;
  color: var(--dim2);
}

.card-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

/* Spans the card once it wraps below the buttons. */
.card-progress {
  flex-basis: 100%;
  height: 4px;
  border-radius: 4px;
  background: var(--panel2);
  overflow: hidden;
}

.card-progress-fill {
  height: 100%;
  background: var(--accent);
  transition: width 200ms ease;
}

/* ── Library folder ───────────────────────────────────────────────────────── */
/* ── Storage locations ── */
.unit-list { padding: 0; overflow: hidden; }

.unit-empty {
  padding: 16px;
  font-size: 12px;
  color: var(--text);

  p { margin: 0; }
}

.unit-empty-hint {
  margin-top: 4px !important;
  color: var(--dim);
  line-height: 1.5;
}

.unit {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--line);

  &:last-child { border-bottom: 0; }
  &.offline { opacity: 0.62; }
}

.unit-rank {
  width: 20px;
  font-size: 11px;
  color: var(--dim);
  text-align: right;
}

.unit-body { flex: 1; min-width: 0; }

.unit-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text);
}

.unit-badge {
  font-size: 10px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--dim);
  border: 1px solid var(--line);
  border-radius: 3px;
  padding: 1px 5px;
}

.unit-path {
  font-size: 11px;
  color: var(--dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.unit-space {
  font-size: 11.5px;
  color: var(--dim);
  white-space: nowrap;
}

.unit-usage {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-top: 1px solid var(--line);
  font-size: 11.5px;
  color: var(--dim);
}

.unit-usage-in { color: var(--dim); }

.setup-units {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  margin: 18px 0;
}

.setup-unit {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 14px;
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  background: var(--panel);
  text-align: left;
}

.setup-unit-name { font-size: 13px; color: var(--text); }
.setup-unit-path { font-size: 11px; color: var(--dim); word-break: break-all; }

.unit-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.btn-icon {
  width: 24px;
  height: 24px;
  border: 1px solid var(--line);
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  color: var(--dim);

  &:hover:not(:disabled) { background: var(--panel2); color: var(--text); }
  &:disabled { opacity: 0.35; }
}

.btn-small { padding: 4px 10px; font-size: 11.5px; }

/* btn-danger is defined in GameDetail's scoped block, so it does not reach here.
   Same treatment, stated once more rather than hoisted — the two screens are the
   only users, and a shared button system is a bigger change than this warrants. */
.btn-outline.btn-danger {
  color: var(--bad);
  &:hover:not(:disabled) { border-color: var(--bad); }
}

.folder-card {
  padding: 16px 18px;
}

.folder-head {
  display: flex;
  align-items: center;
  gap: 12px;
}

.folder-path {
  font-size: 12.5px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.storage {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 14px;
}

/* Two stacked segments over a --panel2 track; the remainder is free space. */
.storage-bar {
  flex: 1;
  display: flex;
  height: 6px;
  border-radius: 6px;
  background: var(--panel2);
  overflow: hidden;
}

.seg {
  height: 100%;
}

.seg-other {
  background: var(--dim2);
}

.seg-library {
  background: var(--accent);
}

.storage-free {
  font-size: 12.5px;
  color: var(--dim);
  white-space: nowrap;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-top: 9px;
  font-size: 11.5px;
  color: var(--dim2);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.swatch {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  flex-shrink: 0;
}

.swatch-library { background: var(--accent); }
.swatch-other { background: var(--dim2); }

.storage-none {
  margin: 12px 0 0;
  font-size: 12.5px;
  color: var(--dim2);
}

/* ── Badges ───────────────────────────────────────────────────────────────── */
.sha {
  padding: 2px 8px;
  border-radius: var(--r-tile);
  border: 1px solid var(--line);
  background: var(--panel2);
  font-size: 11.5px;
  color: var(--dim);
}

.badge-update {
  padding: 3px 9px;
  border-radius: var(--r-pill);
  background: var(--accent-soft);
  color: var(--accent-hi);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.03em;
}

.badge-quiet {
  font-size: 12.5px;
  color: var(--dim2);
}

/* ── Buttons ──────────────────────────────────────────────────────────────── */
.btn-primary {
  padding: 9px 16px;
  border-radius: var(--r-control);
  background: var(--accent);
  color: var(--on-accent);
  font-size: 13px;
  font-weight: 600;

  &:hover:not(:disabled) { background: var(--accent-hi); }
  &:disabled { opacity: 0.5; cursor: default; }
}

.btn-outline {
  padding: 8px 14px;
  border-radius: var(--r-control);
  border: 1px solid var(--line2);
  font-size: 13px;
  font-weight: 500;
  color: var(--text);

  &:hover:not(:disabled) { border-color: var(--dim2); }
  &:disabled { opacity: 0.5; cursor: default; }
}

.error-text {
  margin-top: 16px;
  font-size: 13px;
  color: var(--bad);
}

/* ── First run ────────────────────────────────────────────────────────────── */
.setup-screen {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--pad-page);
}

.setup-content {
  width: 100%;
  max-width: 520px;
}

.setup-title {
  margin: 0;
  font-size: 26px;
  font-weight: 600;
  letter-spacing: -0.02em;
}

.setup-subtitle {
  margin: 10px 0 26px;
  font-size: 14px;
  line-height: 1.65;
  color: var(--dim);
}

.path-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px 8px 14px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-control);

  &:focus-within { border-color: var(--line2); }
}

.path-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: none;
  font-size: 12.5px;
  color: var(--text);

  &::placeholder { color: var(--dim2); font-family: var(--font-ui); }
}

.setup-content .btn-primary {
  margin-top: 22px;
  padding: 11px 26px;
  font-size: 14px;
}
</style>
