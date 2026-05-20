<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  ScanDiscs, GetOpticalDrives, StartDump, CancelDump,
  GetRedumperPath, SetRedumperPath, SelectExecutable,
} from '../../wailsjs/go/main/App'

const drives      = ref([])
const discInfoMap = ref({})  // drive.path → DiscInfo
const dumpMap     = ref({})  // drive.path → DumpProgress
const scanning    = ref(false)

const redumperPath = ref('')

// scanDiscs fetches the current drive list and triggers background probing for
// any disc that is present. On Linux the background watcher handles ongoing
// events; this is the primary detection path on macOS and Windows.
async function scanDiscs() {
  scanning.value = true
  try {
    drives.value = await ScanDiscs() ?? []
  } catch {
    drives.value = []
  } finally {
    scanning.value = false
  }
}

async function loadRedumperPath() {
  redumperPath.value = await GetRedumperPath().catch(() => '')
}

async function browseRedumper() {
  const chosen = await SelectExecutable().catch(() => null)
  if (!chosen) return
  await SetRedumperPath(chosen)
  redumperPath.value = chosen
}

function discLabel(drive) {
  const info = discInfoMap.value[drive.path]
  if (!info) return 'Identifying…'
  const sys = systemLabel(info.system)
  if (info.serial) return sys ? `${sys} · ${info.serial}` : info.serial
  if (info.volume) return sys ? `${sys} · ${info.volume}` : info.volume
  if (sys)         return sys
  return 'Unknown disc'
}

function systemLabel(sys) {
  return {
    psx:     'PlayStation',
    ps2:     'PlayStation 2',
    gamecube:'GameCube',
    wii:     'Wii',
    xbox:    'Xbox',
    xbox360: 'Xbox 360',
  }[sys] ?? ''
}

function dumpImageName(drive) {
  const info = discInfoMap.value[drive.path]
  return (info?.serial || info?.volume || 'DISC')
}

async function startDump(drive) {
  await StartDump(drive.path, dumpImageName(drive))
}

const anyDumping = computed(() =>
  Object.values(dumpMap.value).some(d => d.phase === 'dumping')
)

onMounted(async () => {
  await Promise.all([scanDiscs(), loadRedumperPath()])

  // disc:inserted / disc:ejected are only emitted on Linux (udev watcher).
  // On macOS and Windows the user refreshes manually via the Refresh button.
  EventsOn('disc:inserted', async () => {
    drives.value = await GetOpticalDrives().catch(() => drives.value)
  })
  EventsOn('disc:ejected', async ({ drive }) => {
    drives.value = await GetOpticalDrives().catch(() => drives.value)
    const m = { ...discInfoMap.value }; delete m[drive]; discInfoMap.value = m
    const d = { ...dumpMap.value };     delete d[drive]; dumpMap.value     = d
  })
  EventsOn('disc:identified', info => {
    discInfoMap.value = { ...discInfoMap.value, [info.drive]: info }
  })
  EventsOn('dump:progress', progress => {
    dumpMap.value = { ...dumpMap.value, [progress.drive]: progress }
    if (progress.phase === 'done' || progress.phase === 'error') {
      setTimeout(() => {
        const d = { ...dumpMap.value }; delete d[progress.drive]; dumpMap.value = d
      }, 6000)
    }
  })
})

onUnmounted(() => {
  EventsOff('disc:inserted')
  EventsOff('disc:ejected')
  EventsOff('disc:identified')
  EventsOff('dump:progress')
})
</script>

<template>
  <div class="disc-dumper">

    <!-- toolbar -->
    <div class="toolbar">
      <div class="config-bar">
        <div class="config-bar-left">
          <span class="config-label">redumper</span>
          <span v-if="redumperPath" class="config-path">{{ redumperPath }}</span>
          <span v-else class="config-path muted">not configured</span>
        </div>
        <button class="btn-action" @click="browseRedumper">Browse…</button>
      </div>
      <button class="btn-action" :disabled="scanning" @click="scanDiscs">
        {{ scanning ? 'Scanning…' : 'Refresh' }}
      </button>
    </div>

    <!-- drives -->
    <div v-if="drives.length === 0" class="empty-state">
      <p class="empty-heading">No optical drives detected</p>
      <p class="empty-sub">Connect an optical drive and insert a disc to get started.</p>
    </div>

    <div v-else class="drive-grid">
      <div v-for="drive in drives" :key="drive.path" class="drive-card">

        <div class="drive-top">
          <div class="drive-identity">
            <span class="drive-name">{{ drive.label }}</span>
            <span class="drive-dev">{{ drive.path }}</span>
          </div>
          <div class="disc-badge" :class="drive.hasDisc ? 'present' : 'absent'">
            {{ drive.hasDisc ? 'Disc present' : 'No disc' }}
          </div>
        </div>

        <template v-if="drive.hasDisc">
          <div class="disc-info">
            <span class="disc-label">{{ discLabel(drive) }}</span>
            <span v-if="discInfoMap[drive.path]?.discType" class="disc-type">
              {{ discInfoMap[drive.path].discType.toUpperCase() }}
            </span>
          </div>

          <!-- Dump in progress -->
          <div v-if="dumpMap[drive.path]" class="dump-progress-area">
            <div class="progress-track">
              <div
                class="progress-fill"
                :class="dumpMap[drive.path].phase"
                :style="{ width: dumpMap[drive.path].percent + '%' }"
              />
            </div>
            <div class="progress-meta">
              <span class="progress-label">
                <template v-if="dumpMap[drive.path].phase === 'dumping'">
                  Dumping… {{ dumpMap[drive.path].percent.toFixed(1) }}%
                </template>
                <template v-else-if="dumpMap[drive.path].phase === 'done'">
                  Dump complete
                </template>
                <template v-else>
                  Error: {{ dumpMap[drive.path].error }}
                </template>
              </span>
              <button
                v-if="dumpMap[drive.path].phase === 'dumping'"
                class="btn-cancel"
                @click="CancelDump()"
              >Cancel</button>
            </div>
          </div>

          <!-- Idle -->
          <div v-else class="drive-actions">
            <button
              class="btn-dump"
              :disabled="!redumperPath || anyDumping"
              :title="!redumperPath ? 'Configure redumper path first' : ''"
              @click="startDump(drive)"
            >Dump disc</button>
            <span v-if="!redumperPath" class="action-hint">redumper not configured</span>
          </div>
        </template>

      </div>
    </div>

  </div>
</template>

<style lang="scss" scoped>
.disc-dumper {
  padding: 32px 24px;
  max-width: 800px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ── Toolbar ── */
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
}

.config-bar {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 14px;
  background: #2e2e2e;
  border: 1px solid #4e4e4e;
  border-radius: 6px;
  padding: 10px 14px;
}

.config-bar-left {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.config-label {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.8px;
  color: #666;
  flex-shrink: 0;
}

.config-path {
  font-size: 13px;
  font-family: monospace;
  color: #a0a0a0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  &.muted { color: #555; }
}

/* ── Empty state ── */
.empty-state {
  text-align: center;
  padding: 60px 0;
}

.empty-heading {
  font-size: 16px;
  font-weight: 600;
  color: #8b929a;
  margin: 0 0 8px;
}

.empty-sub {
  font-size: 13px;
  color: #555;
  margin: 0;
}

/* ── Drive cards ── */
.drive-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.drive-card {
  background: #343434;
  border: 1px solid #4e4e4e;
  border-radius: 8px;
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.drive-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.drive-identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.drive-name {
  font-size: 15px;
  font-weight: 600;
  color: #c8c8c8;
}

.drive-dev {
  font-size: 12px;
  font-family: monospace;
  color: #666;
}

.disc-badge {
  font-size: 11px;
  font-weight: 600;
  border-radius: 3px;
  padding: 2px 8px;
  flex-shrink: 0;

  &.present {
    color: #50c878;
    background: rgba(80, 200, 120, 0.12);
    border: 1px solid rgba(80, 200, 120, 0.3);
  }
  &.absent {
    color: #666;
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid #3e3e3e;
  }
}

.disc-info {
  display: flex;
  align-items: center;
  gap: 10px;
}

.disc-label {
  font-size: 14px;
  color: #a0a0a0;
}

.disc-type {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.5px;
  color: #555;
  background: rgba(255,255,255,0.05);
  border: 1px solid #3e3e3e;
  border-radius: 3px;
  padding: 1px 6px;
}

/* ── Dump progress ── */
.dump-progress-area {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.progress-track {
  height: 4px;
  background: #4e4e4e;
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease;

  &.dumping { background: #50c878; }
  &.done    { background: #50c878; }
  &.error   { background: #e06c75; }
}

.progress-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.progress-label {
  font-size: 12px;
  color: #8b929a;
}

.btn-cancel {
  background: none;
  border: 1px solid #4e4e4e;
  border-radius: 4px;
  color: #8b929a;
  font: inherit;
  font-size: 11px;
  padding: 2px 10px;
  cursor: pointer;
  &:hover { color: #e06c75; border-color: #e06c75; }
}

/* ── Drive actions ── */
.drive-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.btn-dump {
  background: #d4d4d4;
  color: #111;
  border: none;
  border-radius: 5px;
  padding: 7px 20px;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;

  &:hover:not(:disabled) { background: #e8e8e8; }
  &:disabled { opacity: 0.35; cursor: default; }
}

.action-hint {
  font-size: 12px;
  color: #555;
}

/* ── Shared buttons ── */
.btn-action {
  background: none;
  border: 1px solid #4e4e4e;
  border-radius: 4px;
  color: #8b929a;
  font: inherit;
  font-size: 12px;
  padding: 3px 12px;
  cursor: pointer;
  white-space: nowrap;

  &:hover { color: #b4b4b4; border-color: #646464; }

  &.primary {
    color: #c8c8c8;
    border-color: #5e5e5e;
    &:hover { border-color: #7e7e7e; color: #e0e0e0; }
  }
}
</style>
