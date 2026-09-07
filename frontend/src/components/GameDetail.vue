<script setup>
import { ref, computed, inject, watch, nextTick } from 'vue'
import { marked } from 'marked'
import {
  GetROMStatus, GetInstallState, GetInstallPrompts,
  GetSpecVersions, GetInstallSize, LaunchVersion, CleanBuildDir, UninstallVersion,
  GetItemUpdate, UpdateMediaItem, SelectROMFiles, AddROMFiles,
} from '../../wailsjs/go/main/App'
import { artworkUrl, ART_WIDTH } from '../lib/artwork'
import { useRomRequirements } from '../composables/useRomRequirements'
import RomRequirements from './RomRequirements.vue'

const props = defineProps({
  game: { type: Object, required: true },
  platform: { type: String, required: true },
})

const emit = defineEmits(['back'])

// Install state is owned by App.vue and shared via provide/inject.
const activeInstall = inject('activeInstall')
const installLog = inject('installLog', ref([]))
const INSTALL_LOG_MAX = inject('installLogMax', 500)
const startInstall = inject('startInstall')
const clearInstall = inject('clearInstall')
const cancelInstall = inject('cancelInstall')

const tab = ref('overview')

const romStatus = ref({})
const romAddError = ref(null)
const addingRom = ref(null) // dep.title while a picker/import is in flight

const installState = ref(null)
const installSize = ref(0)
const updateAvailable = ref(false)
const installError = ref(null)
const showExeMenu = ref(false)
const confirmUninstall = ref(false)

const installPrompts = ref([])
const selectedArgs = ref({})

const specVersions = ref([]) // [{ version, platforms }] in declaration order
const selectedVersion = ref('')
const selectedPlatform = ref('')
const showVersionMenu = ref(false)

// ── Install progress (derived from the shared activeInstall) ────────────────
const isInstalling = computed(() =>
  activeInstall.value?.itemTitle === props.game._itemTitle && !activeInstall.value?.failed
)
const buildFailed = computed(() =>
  activeInstall.value?.itemTitle === props.game._itemTitle ? activeInstall.value.failed : null
)
const otherInstallRunning = computed(() =>
  activeInstall.value != null && activeInstall.value.itemTitle !== props.game._itemTitle
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
  if (stepTotal > 0) return Math.round(((stepIndex + 1) / stepTotal) * 100)
  if (phase === 'downloading') return Math.round(percent * 0.8)
  if (phase === 'extracting') return 85
  if (phase === 'copying_roms') return 95
  return 0
})

// ── ROM requirements ────────────────────────────────────────────────────────
// The rules themselves live in the composable, so this page and the ROM library
// cannot drift apart on what "satisfied" means.
const { matchedOption, unmetRequired } = useRomRequirements(() => romStatus.value)

const requirements = computed(() => props.game.romDependencies ?? [])

const romsReady = computed(() => unmetRequired(requirements.value).length === 0)

const unmetRequirements = computed(() => unmetRequired(requirements.value))

const matchedRom = computed(() => {
  for (const req of requirements.value) {
    const hit = matchedOption(req)
    if (hit) return hit
  }
  return null
})

// Opens a file picker and imports whatever matches this version's ROM
// dependencies. Files are matched by checksum, so a pick is accepted regardless
// of which dependency row the button was pressed on — the button is per-row only
// because that is where a missing ROM is noticed.
async function addRomFiles(dep) {
  romAddError.value = null
  addingRom.value = dep?.title ?? '*'
  try {
    const paths = await SelectROMFiles()
    if (!paths?.length) return
    const matched = await AddROMFiles(props.game._itemTitle, paths, false)
    if (!matched?.length) {
      romAddError.value = 'None of the selected files matched a required ROM for this game.'
    }
    romStatus.value = await GetROMStatus(props.game._itemTitle)
  } catch (e) {
    romAddError.value = String(e)
  } finally {
    addingRom.value = null
  }
}

// The on-screen log is capped, so once it fills, the bare number is the cap
// rather than how much the build has emitted — which reads as a coincidence
// that it always says exactly 500. Say what it is, and say where the rest went.
const logCountLabel = computed(() =>
  installLog.value.length >= INSTALL_LOG_MAX
    ? `last ${INSTALL_LOG_MAX} lines`
    : `${installLog.value.length} line${installLog.value.length === 1 ? '' : 's'}`
)

// ── Versions ────────────────────────────────────────────────────────────────
// The selector is pointless with a single version, and actively misleading if
// some objects in .install.json declare a version and others do not.
const showVersionPicker = computed(() => specVersions.value.length > 1)

const versionsIncomplete = computed(() =>
  specVersions.value.length > 1 && specVersions.value.some(v => !v.version)
)

const currentVersion = computed(() =>
  specVersions.value.find(v => v.version === selectedVersion.value) ?? specVersions.value[0] ?? null
)

const platformOptions = computed(() => currentVersion.value?.platforms ?? [])

// Building for another platform is supported, so the list is not filtered to the
// host; the host is only the default when this version can target it.
//
// The host platform carries its architecture ("Mac-arm64") while a spec may
// declare either that or the bare OS ("Mac"), so an exact match is preferred and
// the bare OS accepted next — a port shipping separate arm64 and x64 builds then
// defaults to the right one. Mirrors resolveTargetPlatform in app.go.
function defaultPlatformFor(version) {
  const platforms = version?.platforms ?? []
  if (!platforms.length) return ''
  const base = props.platform.split('-')[0]
  return platforms.find(p => p === props.platform)
      ?? platforms.find(p => p === base)
      ?? platforms[0]
}

function selectVersion(v) {
  showVersionMenu.value = false
  selectedVersion.value = v.version
  selectedPlatform.value = defaultPlatformFor(v)
}

// Release notes for the selected version, matched by title. Absent for versions
// the catalog has no notes for, in which case the card is hidden.
const releaseNotes = computed(() => {
  const notes = props.game.versions ?? []
  if (!notes.length) return null
  if (!selectedVersion.value) return notes[0]
  return notes.find(n => n.title === selectedVersion.value) ?? null
})

const releaseNotesHtml = computed(() =>
  releaseNotes.value?.content ? marked.parse(releaseNotes.value.content) : ''
)

// GetSpecVersions returns newest first, so the head of the list is the latest.
const latestVersion = computed(() => specVersions.value[0]?.version ?? '')

// A port is buildable if it declares an install spec at all. Which platform it is
// built for is the user's choice from the picker, not a filter on the host —
// PortForge builds for platforms it does not run on. A version listing no
// platforms is unrestricted; it installs with an empty target that Go fills in
// with the host.
const buildable = computed(() => specVersions.value.length > 0)

// ── Primary action ──────────────────────────────────────────────────────────
const canInstall = computed(() => {
  if (!romsReady.value || otherInstallRunning.value || !buildable.value) return false
  for (const prompt of installPrompts.value) {
    if (!selectedArgs.value[prompt.name]) return false
  }
  return true
})

const primaryExecutable = computed(() => installState.value?.executables?.[0] ?? null)
const extraExecutables = computed(() => installState.value?.executables?.slice(1) ?? [])

// The reason the primary button is unavailable, shown as its tooltip.
const primaryBlockedReason = computed(() => {
  if (!buildable.value) return 'The catalog has no install spec for this port.'
  if (!romsReady.value) return 'This port needs a ROM you do not have yet.'
  if (otherInstallRunning.value) return `Installing ${activeInstall.value.itemTitle}. Only one install runs at a time.`
  if (installPrompts.value.some(p => !selectedArgs.value[p.name])) return 'Choose the build options first, under Options.'
  return ''
})

// ── Loading ─────────────────────────────────────────────────────────────────
async function loadState() {
  const [romResult, stateResult, promptResult, updateResult, versionResult, sizeResult] =
    await Promise.allSettled([
      GetROMStatus(props.game._itemTitle),
      GetInstallState(props.game._itemTitle),
      GetInstallPrompts(props.game._itemTitle),
      GetItemUpdate(props.game._itemTitle),
      GetSpecVersions(props.game._itemTitle),
      GetInstallSize(props.game._itemTitle),
    ])

  const value = (r, fallback) => (r.status === 'fulfilled' ? r.value ?? fallback : fallback)

  romStatus.value = value(romResult, {})
  romAddError.value = null
  installState.value = value(stateResult, null)
  installSize.value = value(sizeResult, 0)
  updateAvailable.value = value(updateResult, false)
  installPrompts.value = value(promptResult, [])
  specVersions.value = value(versionResult, [])

  // Prefer the version already installed so the page opens describing what the
  // user actually has. Otherwise take the one the spec file marks as default,
  // which is not always the newest — a project whose newest release is a
  // pre-release marks the stable one instead. The list is newest first, so the
  // final fallback is still a sensible choice.
  const installedVersion = installState.value?.installedVersion
  const initial =
    specVersions.value.find(v => v.version === installedVersion)
    ?? specVersions.value.find(v => v.default)
    ?? specVersions.value[0]
    ?? null
  selectedVersion.value = initial?.version ?? ''
  selectedPlatform.value = installState.value?.targetPlatform || defaultPlatformFor(initial)

  // Seed args from the installed build when there is one, so Options opens
  // showing what this copy was made with rather than defaults.
  const newArgs = { ...(installState.value?.args ?? {}) }
  for (const prompt of installPrompts.value) {
    if (!newArgs[prompt.name] && prompt.type === 'choice' && prompt.options?.length) {
      newArgs[prompt.name] = prompt.options[0].value
    }
  }
  selectedArgs.value = newArgs

  tab.value = 'overview'
}

watch(() => props.game, loadState, { immediate: true })

// When a background install for this game finishes, refresh so Play appears.
watch(activeInstall, (cur, prev) => {
  if (prev?.itemTitle === props.game._itemTitle && cur === null) loadState()
})

// ── Build output ────────────────────────────────────────────────────────────
// Collapsed by default: a successful build's output is noise. A failure is the
// one moment it is the first thing worth reading, so open it automatically.
const showLog = ref(false)
const logEl = ref(null)

watch(buildFailed, failed => { if (failed) showLog.value = true })

// Follow the tail while it is open, so a long compile keeps showing its newest
// line without the user chasing the scrollbar.
watch(() => installLog.value.length, async () => {
  if (!showLog.value) return
  await nextTick()
  if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
})

watch(showLog, async open => {
  if (!open) return
  await nextTick()
  if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
})

// ── Actions ─────────────────────────────────────────────────────────────────
async function install() {
  installError.value = null
  const ok = await startInstall(
    props.game._itemTitle, selectedArgs.value, selectedVersion.value, selectedPlatform.value
  )
  if (ok) await loadState()
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

async function uninstall() {
  confirmUninstall.value = false
  try {
    await UninstallVersion(props.game._itemTitle)
    await loadState()
  } catch (e) {
    installError.value = String(e)
  }
}

async function update() {
  installError.value = null
  try {
    await UpdateMediaItem(props.game._itemTitle)
    updateAvailable.value = false
  } catch (e) {
    installError.value = String(e)
  }
}

// ── Presentation helpers ────────────────────────────────────────────────────

function artworkOf(type) {
  return props.game.artwork?.find(a => a.artworkType.toLowerCase() === type.toLowerCase()) ?? null
}

// Each artwork is requested at the width it is drawn at, so the same file can be
// served small here and large there. See src/lib/artwork.js.
function artSrc(art, width) {
  if (!art) return null
  return artworkUrl(props.game, art.fileName, width)
}

const coverArt = computed(() => artworkOf('cover') ?? props.game.artwork?.[0])

const coverSrc = computed(() => artSrc(coverArt.value, ART_WIDTH.heroCover))
const heroSrc = computed(() => artSrc(artworkOf('hero'), ART_WIDTH.hero()))
// The stand-in backdrop is blurred past the point where detail survives, so it
// is fetched at its own small size instead of reusing the full-resolution cover.
const heroFallbackSrc = computed(() => artSrc(coverArt.value, ART_WIDTH.heroFallback))

const screenshots = computed(() =>
  (props.game.artwork ?? [])
    .filter(a => a.artworkType.toLowerCase() === 'screenshot')
    .map(a => ({
      name: a.fileName,
      thumb: artSrc(a, ART_WIDTH.screenshot),
      // The lightbox is the one place the original is worth its decode cost.
      full: artSrc(a),
    }))
)

const lightbox = ref(null)

const descriptionHtml = computed(() => marked.parse(props.game.description ?? ''))

// Tag pills: the catalog's own tags, with the version type appended as one more
// since it reads the same way ("Harbour Masters", "Zelda", "Fan port").
const tagPills = computed(() => {
  const pills = [...(props.game.tags ?? [])]
  if (props.game.versionType) pills.push(humanise(props.game.versionType))
  return pills
})

function humanise(s) {
  return String(s).replace(/([a-z])([A-Z])/g, '$1 $2')
}

const subline = computed(() => {
  const parts = []
  const g = props.game.videoGame
  if (g) parts.push(g.title || g._itemTitle)
  if (g?.releaseYear) parts.push(String(g.releaseYear))
  if (props.game.versionType) parts.push(humanise(props.game.versionType))
  return parts.join(' · ')
})

const statusText = computed(() => {
  if (isInstalling.value) return 'Building'
  if (buildFailed.value) return 'Build failed'
  if (updateAvailable.value) return 'Update ready'
  if (installState.value?.installed) return 'Installed'
  if (!buildable.value) return 'Unavailable'
  return 'Not installed'
})

// The label is the platform a dump came off, not the type name. The fallback
// strips the medium+form suffix so a type added to the catalog before it is
// added here still reads sensibly rather than blank.
function formatSize(bytes) {
  if (!bytes) return '—'
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return bytes + ' B'
}

function formatPlaytime(secs) {
  if (!secs) return null
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function formatDate(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}
</script>

<template>
  <div class="detail">
    <!-- Back bar -->
    <div class="back-bar">
      <button class="back-btn" @click="emit('back')">
        <span class="chevron-left" aria-hidden="true" />
        Library
      </button>
    </div>

    <div class="detail-scroll">
      <!-- Key art header -->
      <header class="hero" :class="{ 'art-placeholder art-placeholder--wide': !heroSrc && !coverSrc }">
        <img v-if="heroSrc" :src="heroSrc" class="hero-art" alt="" />
        <!-- With no key art, the cover stands in as a blurred backdrop rather
             than leaving the largest element on the page empty. -->
        <img v-else-if="heroFallbackSrc" :src="heroFallbackSrc" class="hero-art hero-art-fallback" alt="" />
        <span v-else class="art-placeholder__label">KEY ART</span>
        <div class="hero-scrim" />

        <div class="hero-content">
          <div class="hero-cover" :class="{ 'art-placeholder': !coverSrc }">
            <img v-if="coverSrc" :src="coverSrc" :alt="game.title || game._itemTitle" />
            <span v-else class="art-placeholder__label">COVER</span>
          </div>

          <div class="hero-text">
            <div v-if="tagPills.length" class="hero-tags">
              <span v-for="t in tagPills" :key="t" class="pill">{{ t }}</span>
            </div>
            <h1 class="hero-title">{{ game.title || game._itemTitle }}</h1>
            <p v-if="subline" class="hero-subline">{{ subline }}</p>
          </div>

          <div class="hero-actions">
            <!-- Version and target platform. Both are hidden when there is
                 nothing to choose between. -->
            <div v-if="showVersionPicker" class="picker">
              <button class="picker-btn mono" @click="showVersionMenu = !showVersionMenu">
                {{ selectedVersion || 'Select version' }}
                <span class="chevron-down" aria-hidden="true" />
              </button>
              <ul v-if="showVersionMenu" class="picker-menu">
                <li v-for="v in specVersions" :key="v.version || '?'">
                  <button class="mono" :class="{ selected: v.version === selectedVersion }" @click="selectVersion(v)">
                    {{ v.version || 'Unnamed version' }}
                  </button>
                </li>
              </ul>
            </div>

            <select v-if="platformOptions.length > 1" v-model="selectedPlatform" class="picker-btn mono platform-select">
              <option v-for="p in platformOptions" :key="p" :value="p">{{ p }}</option>
            </select>

            <!-- Primary action -->
            <div v-if="installState?.installed && !isInstalling" class="play-group">
              <button class="btn-primary" @click="launch(primaryExecutable?.path ?? '')">
                {{ primaryExecutable?.title ?? 'Play' }}
              </button>
              <button
                v-if="extraExecutables.length"
                class="btn-primary btn-play-more"
                title="More executables"
                @click="showExeMenu = !showExeMenu"
              >
                <span class="chevron-down" aria-hidden="true" />
              </button>
              <ul v-if="showExeMenu" class="picker-menu exe-menu">
                <li v-for="exe in extraExecutables" :key="exe.path">
                  <button @click="launch(exe.path)">{{ exe.title || exe.path }}</button>
                </li>
              </ul>
            </div>

            <button v-else-if="isInstalling" class="btn-primary btn-busy" @click="cancelInstall()">
              Stop
            </button>

            <button
              v-else
              class="btn-primary"
              :disabled="!canInstall"
              :title="primaryBlockedReason"
              @click="install()"
            >Install</button>
          </div>
        </div>
      </header>

      <!-- Tabs -->
      <nav class="tabs">
        <button class="tab" :class="{ active: tab === 'overview' }" @click="tab = 'overview'">Overview</button>
        <button
          v-if="installPrompts.length"
          class="tab"
          :class="{ active: tab === 'options' }"
          @click="tab = 'options'"
        >Options</button>
      </nav>

      <div class="body">
        <div class="body-main">
          <!-- Install progress and failure both belong at the top of the page:
               they are what the user came back to check. -->
          <div v-if="isInstalling" class="progress-card">
            <div class="progress-head">
              <span class="progress-label">{{ progressLabel }}</span>
              <span class="mono progress-pct">{{ progressPercent }}%</span>
            </div>
            <div class="progress-track">
              <div class="progress-fill" :style="{ width: progressPercent + '%' }" />
            </div>
          </div>

          <div v-if="buildFailed" class="alert alert-bad">
            <span class="icon-cross" aria-hidden="true" />
            <div class="alert-body">
              <p class="alert-title">Build failed at <code>{{ buildFailed.step }}</code></p>
              <p class="alert-text">{{ buildFailed.error }}</p>
              <p class="alert-text">Full output was saved to <code>install.log</code> in the game's data folder.</p>
              <div class="alert-actions">
                <button class="btn-outline btn-danger" @click="cleanBuild">Delete build folder</button>
                <button class="btn-outline" @click="clearInstall()">Keep it</button>
              </div>
            </div>
          </div>

          <!-- Live build output. Present during the build and kept on screen after
               a failure, which is when it is actually read. -->
          <div v-if="installLog.length && (isInstalling || buildFailed)" class="log-card">
            <button class="log-toggle" @click="showLog = !showLog" :aria-expanded="showLog">
              <span class="log-caret" :class="{ open: showLog }" aria-hidden="true" />
              <span class="log-toggle-text">{{ showLog ? 'Hide' : 'Show' }} build output</span>
              <span class="log-count mono">{{ logCountLabel }}</span>
            </button>
            <pre v-if="showLog" ref="logEl" class="log-body mono"><span
              v-for="(entry, i) in installLog"
              :key="i"
              class="log-line"
              :class="{ 'log-line-err': entry.stream === 'stderr' }"
            >{{ entry.line }}</span></pre>
          </div>

          <p v-if="installError" class="alert-text error-text">{{ installError }}</p>

          <p v-if="versionsIncomplete" class="alert alert-warn">
            <span class="icon-warn" aria-hidden="true" />
            <span class="alert-text">
              This port declares several builds but not all of them name a version, so the
              list below may be incomplete. The catalog entry needs a <code>version</code>
              on every object in its <code>.install.json</code>.
            </span>
          </p>

          <template v-if="tab === 'overview'">
            <div v-if="game.description" class="description" v-html="descriptionHtml" />

            <section v-if="screenshots.length" class="section">
              <h2 class="eyebrow">Screenshots</h2>
              <div class="shots">
                <button
                  v-for="(shot, i) in screenshots"
                  :key="shot.name"
                  class="shot"
                  @click="lightbox = shot.full"
                >
                  <img :src="shot.thumb" :alt="`Screenshot ${i + 1}`" />
                </button>
              </div>
            </section>

            <section v-if="releaseNotes" class="section">
              <h2 class="eyebrow">
                What's new in {{ releaseNotes.title }}
                <span v-if="releaseNotes.date" class="notes-date">{{ releaseNotes.date }}</span>
              </h2>
              <div class="card notes" v-html="releaseNotesHtml" />
            </section>

            <!-- ROM requirement. Hidden entirely once it is met: the right column
                 reports the satisfied state, and a wall of checksums is noise
                 when there is nothing to act on. -->
            <section v-if="requirements.length && !romsReady" class="section">
              <h2 class="eyebrow">{{ requirements.length === 1 ? 'ROM requirement' : 'ROM requirements' }}</h2>

              <div class="alert alert-bad rom-alert">
                <span class="icon-cross" aria-hidden="true" />
                <div class="alert-body">
                  <p class="alert-title">
                    {{ unmetRequirements.length === 1
                        ? 'No matching ROM found in your library'
                        : `${unmetRequirements.length} of this port's requirements are unmet` }}
                  </p>
                  <p class="alert-text">
                    This port needs
                    <template v-if="requirements.filter(r => r.required).length > 1">
                      one file for each of the {{ requirements.filter(r => r.required).length }}
                      requirements below — any listed alternative will do for each.
                    </template>
                    <template v-else>one of the files below.</template>
                    PortForge matches them by checksum, not by name — drop one onto the
                    window, or add it here.
                  </p>
                </div>
              </div>

              <RomRequirements
                :requirements="requirements"
                :status="romStatus"
                footer
                expand-unmet
                :adding="addingRom !== null"
                @add="addRomFiles(null)"
              />

              <p v-if="romAddError" class="alert-text error-text">{{ romAddError }}</p>
            </section>
          </template>

          <template v-else-if="tab === 'options'">
            <section class="section">
              <h2 class="eyebrow">Build options</h2>
              <p class="options-hint">
                These are chosen before installing and baked into the build. Changing one
                means rebuilding.
              </p>
              <div class="option-card card" v-for="prompt in installPrompts" :key="prompt.name">
                <div class="option-label">
                  <span class="option-name">{{ prompt.label || prompt.name }}</span>
                </div>
                <div v-if="prompt.type === 'choice'" class="option-choices">
                  <button
                    v-for="opt in prompt.options"
                    :key="opt.value"
                    class="chip"
                    :class="{ active: selectedArgs[prompt.name] === opt.value }"
                    @click="selectedArgs[prompt.name] = opt.value"
                  >{{ opt.label }}</button>
                </div>
                <input v-else v-model="selectedArgs[prompt.name]" class="option-input mono" type="text" />
              </div>
            </section>
          </template>
        </div>

        <!-- Right column -->
        <aside class="body-side">
          <section class="card">
            <h2 class="eyebrow">Installation</h2>

            <div v-if="requirements.length" class="rom-status" :class="{ met: romsReady }">
              <span v-if="romsReady" class="icon-check" aria-hidden="true" />
              <span v-else class="icon-cross-sm bad" aria-hidden="true" />
              <div class="rom-status-text">
                <span class="rom-status-title">
                  {{ romsReady ? 'ROM requirement met' : 'ROM required' }}
                </span>
                <span v-if="romsReady && matchedRom" class="mono rom-status-detail">
                  {{ matchedRom.title }}
                </span>
                <span v-else-if="!romsReady" class="rom-status-detail">
                  {{ unmetRequirements.length === requirements.filter(r => r.required).length
                      ? 'None of the accepted files are in your library yet.'
                      : `${unmetRequirements.length} of ${requirements.filter(r => r.required).length} requirements still unmet.` }}
                </span>
              </div>
            </div>

            <dl class="facts">
              <div class="fact"><dt>Status</dt><dd>{{ statusText }}</dd></div>
              <div v-if="installState?.installedVersion" class="fact">
                <dt>Installed version</dt><dd class="mono">{{ installState.installedVersion }}</dd>
              </div>
              <div v-if="latestVersion" class="fact">
                <dt>Latest version</dt><dd class="mono">{{ latestVersion }}</dd>
              </div>
              <div v-if="installState?.installed" class="fact">
                <dt>Size on disk</dt><dd class="mono">{{ formatSize(installSize) }}</dd>
              </div>
              <div v-if="installState?.installDir" class="fact">
                <dt>Location</dt><dd class="mono truncate" :title="installState.installDir">{{ installState.installDir }}</dd>
              </div>
              <div v-if="installState?.lastPlayedAt" class="fact">
                <dt>Last played</dt><dd class="mono">{{ formatDate(installState.lastPlayedAt) }}</dd>
              </div>
              <div v-if="formatPlaytime(installState?.totalPlaySeconds)" class="fact">
                <dt>Play time</dt><dd class="mono">{{ formatPlaytime(installState.totalPlaySeconds) }}</dd>
              </div>
            </dl>

            <div v-if="updateAvailable" class="update-row">
              <p class="alert-text">The catalog entry for this port has changed since you installed it.</p>
              <button class="btn-outline" @click="update()">Update entry</button>
            </div>

            <div v-if="installState?.installed" class="card-foot">
              <button class="btn-outline" :disabled="!canInstall" :title="primaryBlockedReason" @click="install()">
                Rebuild
              </button>
              <button v-if="!confirmUninstall" class="btn-outline btn-danger" @click="confirmUninstall = true">
                Uninstall
              </button>
              <template v-else>
                <button class="btn-outline btn-danger" @click="uninstall()">Confirm</button>
                <button class="btn-outline" @click="confirmUninstall = false">Cancel</button>
              </template>
            </div>
          </section>

          <section v-if="game.platforms?.length" class="card">
            <h2 class="eyebrow">Platforms</h2>
            <div class="platform-pills">
              <span
                v-for="p in game.platforms"
                :key="p"
                class="pill"
                :class="{ dim: p !== platform }"
              >{{ p }}</span>
            </div>
          </section>
        </aside>
      </div>
    </div>

    <!-- Screenshot lightbox -->
    <div v-if="lightbox" class="lightbox" @click="lightbox = null">
      <img :src="lightbox" alt="" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.detail {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.back-bar {
  flex-shrink: 0;
  padding: 13px var(--pad-page);
  background: var(--bg);
  border-bottom: 1px solid var(--line);
  z-index: 3;
}

.back-btn {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  font-size: 13px;
  font-weight: 500;
  color: var(--dim);

  &:hover { color: var(--text); }
}

.detail-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* ── Key art header ── */
.hero {
  position: relative;
  height: 380px;
  overflow: hidden;
}

.hero-art {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center 30%;
}

/* Standing in for missing key art: blown up and blurred so it reads as a
   backdrop rather than a stretched cover. */
.hero-art-fallback {
  filter: blur(28px) saturate(1.2);
  transform: scale(1.25);
}

.hero-scrim {
  position: absolute;
  inset: 0;
  background: var(--scrim);
}

.hero-content {
  position: absolute;
  left: 32px;
  right: 32px;
  bottom: 26px;
  display: flex;
  align-items: flex-end;
  gap: 24px;
}

.hero-cover {
  width: 150px;
  flex-shrink: 0;
  border: 1px solid var(--line2);
  border-radius: var(--r-cover);
  box-shadow: var(--shadow);
  overflow: hidden;

  /* Letterboxed to the catalog's 2:3, matching the library grid. */
  aspect-ratio: 2 / 3;
  position: relative;
  img { width: 100%; height: 100%; object-fit: contain; background: var(--panel2); display: block; }
}

.hero-text {
  flex: 1;
  min-width: 0;
}

.hero-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}

.pill {
  padding: 4px 10px;
  border-radius: var(--r-pill);
  background: var(--panel2);
  border: 1px solid var(--line);
  font-size: 11.5px;
  font-weight: 500;
  color: var(--dim);

  &.dim { color: var(--dim2); }
}

.hero-title {
  margin: 0;
  font-size: 40px;
  font-weight: 600;
  letter-spacing: -0.03em;
  line-height: 1;
  color: var(--text);
  text-wrap: pretty;
}

.hero-subline {
  margin: 8px 0 0;
  font-size: 14px;
  color: var(--dim);
}

.hero-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

/* ── Pickers and buttons ── */
.picker {
  position: relative;
}

.picker-btn {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  padding: 10px 14px;
  background: var(--panel);
  border: 1px solid var(--line2);
  border-radius: 10px;
  font-size: 12px;
  color: var(--text);

  &:hover { border-color: var(--dim2); }
}

.platform-select {
  appearance: none;
  cursor: pointer;
}

.picker-menu {
  position: absolute;
  right: 0;
  bottom: calc(100% + 6px);
  z-index: 5;
  min-width: 100%;
  max-height: 260px;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 4px;
  background: var(--panel);
  border: 1px solid var(--line2);
  border-radius: 10px;
  box-shadow: var(--shadow);

  button {
    display: block;
    width: 100%;
    text-align: left;
    white-space: nowrap;
    padding: 8px 10px;
    border-radius: 7px;
    font-size: 12.5px;
    color: var(--text);

    &:hover { background: var(--panel2); }
    &.selected { color: var(--accent); }
  }
}

.btn-primary {
  padding: 13px 40px;
  border-radius: 10px;
  background: var(--accent);
  color: var(--on-accent);
  font-size: 15px;
  font-weight: 600;
  box-shadow: var(--shadow);

  &:hover:not(:disabled) { background: var(--accent-hi); }
  &:disabled { opacity: 0.45; cursor: default; }
}

.play-group {
  position: relative;
  display: flex;
  gap: 2px;
}

.btn-play-more {
  padding: 13px 14px;
  display: flex;
  align-items: center;
  color: var(--on-accent);
}

.exe-menu { min-width: 180px; }

.btn-busy {
  background: var(--panel);
  color: var(--text);
  border: 1px solid var(--line2);
  box-shadow: none;

  &:hover:not(:disabled) { background: var(--panel2); border-color: var(--bad); color: var(--bad); }
}

.btn-outline {
  padding: 9px 14px;
  border: 1px solid var(--line2);
  border-radius: var(--r-control);
  font-size: 13px;
  font-weight: 500;
  color: var(--text);

  &:hover:not(:disabled) { border-color: var(--dim2); }
  &:disabled { opacity: 0.45; cursor: default; }

  &.btn-danger {
    color: var(--bad);
    &:hover:not(:disabled) { border-color: var(--bad); }
  }
}

/* ── Tabs ── */
.tabs {
  display: flex;
  gap: 26px;
  padding: 0 32px;
  border-bottom: 1px solid var(--line);
}

.tab {
  padding: 15px 0 13px;
  margin-bottom: -1px;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--dim);
  border-bottom: 2px solid transparent;

  &:hover { color: var(--text); }
  &.active { color: var(--text); border-bottom-color: var(--accent); }
}

/* ── Body ── */
.body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 30px;
  align-items: start;
  padding: 26px 32px 44px;
}

@media (max-width: 1100px) {
  .body { grid-template-columns: minmax(0, 1fr); }
}

.body-main {
  display: flex;
  flex-direction: column;
  gap: 26px;
  min-width: 0;
}

.body-side {
  display: flex;
  flex-direction: column;
  gap: 14px;
  position: sticky;
  top: 0;
}

.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  padding: 17px 18px;
}

.section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.description {
  font-size: 15px;
  line-height: 1.65;
  max-width: 68ch;
  text-wrap: pretty;
  color: var(--text);

  :deep(p) { margin: 0 0 12px; }
  :deep(p:last-child) { margin-bottom: 0; }
  :deep(a) { color: var(--accent); }
}

.notes-date {
  margin-left: 8px;
  font-weight: 400;
  letter-spacing: 0;
  text-transform: none;
  color: var(--dim2);
}

.notes {
  font-size: 13.5px;
  line-height: 1.5;

  :deep(ul) { margin: 0; padding-left: 18px; }
  :deep(li) { margin-bottom: 6px; }
  :deep(li:last-child) { margin-bottom: 0; }
  :deep(p) { margin: 0 0 10px; }
  :deep(p:last-child) { margin-bottom: 0; }
}

/* ── Screenshots ── */
.shots {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.shot {
  border: 1px solid var(--line);
  border-radius: var(--r-shot);
  overflow: hidden;
  aspect-ratio: 16 / 9;
  /* Transform only — see the note on .cover in GameLibrary. */
  transition: transform 140ms ease;

  img { width: 100%; height: 100%; object-fit: cover; display: block; }
  &:hover { transform: translateY(-2px); }
}

.lightbox {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 5vh 5vw;
  background: rgba(0, 0, 0, 0.82);
  cursor: zoom-out;

  img { max-width: 100%; max-height: 100%; border-radius: var(--r-shot); }
}

/* ── Alerts and progress ── */
.alert {
  display: flex;
  gap: 12px;
  padding: 14px 16px;
  background: var(--panel);
  border: 1px solid var(--line2);
  border-radius: 11px;
}

.alert-warn { border-color: var(--warn); align-items: center; }

.alert-body { min-width: 0; }

.alert-title {
  margin: 0 0 4px;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text);
}

.alert-text {
  margin: 0 0 4px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--dim);

  code { font-family: var(--font-mono); font-size: 11.5px; }
}

.error-text { color: var(--bad); }

.alert-actions {
  display: flex;
  gap: 8px;
  margin-top: 10px;
}

.progress-card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  padding: 15px 18px;
}

.progress-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 10px;
}

.progress-label { flex: 1; font-size: 13px; color: var(--text); }
.progress-pct { font-size: 12px; color: var(--dim); }

.progress-track {
  height: 4px;
  border-radius: 4px;
  background: var(--panel2);
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--accent);
  transition: width 200ms ease;
}

/* ── Build output ── */
.log-card {
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  background: var(--panel);
  overflow: hidden;
}

.log-toggle {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 9px 14px;
  text-align: left;
  font-size: 12px;
  color: var(--dim);

  &:hover { background: var(--panel2); }
}

.log-toggle-text { flex: 1; }

.log-caret {
  width: 0;
  height: 0;
  border-left: 4px solid currentColor;
  border-top: 3.5px solid transparent;
  border-bottom: 3.5px solid transparent;
  transition: transform 120ms ease;

  &.open { transform: rotate(90deg); }
}

.log-count { font-size: 11px; color: var(--dim); }

.log-body {
  display: flex;
  flex-direction: column;
  max-height: 260px;
  margin: 0;
  padding: 10px 14px;
  border-top: 1px solid var(--line);
  background: var(--panel2);
  overflow: auto;
  font-size: 11.5px;
  line-height: 1.55;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-word;
}

.log-line-err { color: var(--bad); }

/* ── ROM requirement ── */
.rom-alert { margin-bottom: 2px; }














/* ── Right column ── */
.rom-status {
  display: flex;
  gap: 10px;
  padding: 11px 12px;
  margin: 12px 0;
  background: var(--panel2);
  border-radius: 10px;
}

.rom-status-text { display: flex; flex-direction: column; gap: 3px; min-width: 0; }

.rom-status-title { font-size: 12.5px; font-weight: 500; color: var(--text); }

.rom-status-detail {
  font-size: 11px;
  line-height: 1.45;
  color: var(--dim);
  word-break: break-word;
}

.facts { margin: 0; }

.fact {
  display: flex;
  justify-content: space-between;
  gap: 14px;
  padding: 7px 0;

  dt { font-size: 13px; color: var(--dim); flex-shrink: 0; }
  dd {
    margin: 0;
    font-size: 12px;
    font-weight: 500;
    color: var(--text);
    text-align: right;
    min-width: 0;
  }
}

.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.update-row {
  padding-top: 12px;
  border-top: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}

.card-foot {
  display: flex;
  gap: 8px;
  margin-top: 14px;

  .btn-outline { flex: 1; text-align: center; padding: 9px 0; }
}

.platform-pills { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }

/* ── Options ── */
.options-hint {
  margin: 0;
  font-size: 12.5px;
  color: var(--dim2);
  max-width: 60ch;
}

.option-card {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 15px 18px;
}

.option-label { flex: 1; min-width: 0; }
.option-name { font-size: 13.5px; font-weight: 500; color: var(--text); }

.option-choices { display: flex; flex-wrap: wrap; gap: 8px; }

.chip {
  padding: 6px 13px;
  border-radius: var(--r-pill);
  border: 1px solid var(--line);
  background: var(--panel2);
  color: var(--dim);
  font-size: 12.5px;
  font-weight: 500;

  &:hover { color: var(--text); }
  &.active { background: var(--accent-soft); border-color: var(--line2); color: var(--accent-hi); }
}

.option-input {
  padding: 7px 12px;
  background: var(--panel2);
  border: 1px solid var(--line);
  border-radius: var(--r-tile);
  font-size: 12px;
  color: var(--text);
  outline: none;

  &:focus { border-color: var(--line2); }
}
</style>
