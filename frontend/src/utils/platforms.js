/**
 * ROM platform definitions shared between the platform tiles (RomPlatforms)
 * and the per-platform emulator settings (Settings).
 *
 * gamesItemType is the MediaItem _itemType for ROMs of that platform (e.g. "NESRom"),
 * also used as the key into Settings.emulators.
 */

export const gamingPlatforms = [
  {
    title:         'Nintendo Entertainment System',
    logoPath:      '/platforms/nes-logo.png',
    backdropPath:  '/platforms/nes-banner.png',
    gamesItemType: 'NESRom',
  }, {
    title:         'Nintendo 64',
    logoPath:      '/platforms/n64-logo.png',
    backdropPath:  '/platforms/n64-banner.png',
    gamesItemType: 'N64Rom',
  }, {
    title:         'Xbox 360',
    logoPath:      '/platforms/xbox360-logo.png',
    backdropPath:  '/platforms/xbox360-banner.png',
    gamesItemType: 'Xbox360Rom',
  }, {
    title:         'PlayStation 1',
    logoPath:      '/platforms/ps1-logo.png',
    backdropPath:  '/platforms/ps1-banner.png',
    gamesItemType: 'PS1Rom',
  },
]
