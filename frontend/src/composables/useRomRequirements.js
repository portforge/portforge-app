// The rules a port's romDependencies obey, in one place.
//
// A port declares one requirement per thing it needs. The options inside a
// requirement are alternatives — the regional and revision variants of one game,
// or the five regional dumps of one disc — so any single present dump satisfies
// it. Requirements themselves combine with AND, which is how a three-disc game
// asks for each disc without enumerating the valid combinations.
//
// Both the game page and the ROM library apply these rules, and they must never
// disagree: a port the game page calls ready while the ROM library shows a red
// cross would leave the user with no way to tell which is lying.

/**
 * @param {() => Record<string, boolean>} status
 *   Reads the MD5 → present map. A getter rather than the map itself so callers
 *   can pass a ref's value and stay reactive.
 */
export function useRomRequirements(status) {
  // Returns true when the user has this dump, false when they demonstrably do
  // not, and null when the answer is not known yet — an option whose formats
  // have not loaded, or a checksum missing from the status map. Null is not
  // false: showing a red cross before the scan has answered would tell the user
  // a file is missing when nothing has looked for it.
  function optionPresent(option) {
    if (!option?.formats?.length) return null
    const results = option.formats.map(f => status()[f.checksums?.md5])
    if (results.some(r => r === true)) return true
    if (results.some(r => r === undefined)) return null
    return false
  }

  function formatPresent(format) {
    return status()[format?.checksums?.md5] === true
  }

  function requirementMet(req) {
    return (req?.options ?? []).some(o => optionPresent(o) === true)
  }

  // The option that actually satisfies a requirement, or null. Useful for
  // showing which dump was matched rather than only that one was.
  function matchedOption(req) {
    return (req?.options ?? []).find(o => optionPresent(o) === true) ?? null
  }

  // Only required requirements hold a port back. A port whose requirements are
  // all optional installs and launches without any of them — newer ports extract
  // their assets on first run rather than at build time, so an unmet optional
  // requirement is a missing extra, not a broken port.
  function unmetRequired(requirements) {
    return (requirements ?? []).filter(r => r.required && !requirementMet(r))
  }

  function isReady(requirements) {
    return unmetRequired(requirements).length === 0
  }

  // Progress over the requirements that matter, for a "2 of 3" style summary.
  function readiness(requirements) {
    const required = (requirements ?? []).filter(r => r.required)
    return { met: required.filter(requirementMet).length, total: required.length }
  }

  // Matched options first, so a satisfied requirement reads at a glance.
  function sortedOptions(req) {
    return [...(req?.options ?? [])].sort(
      (a, b) => (optionPresent(b) === true) - (optionPresent(a) === true)
    )
  }

  return {
    optionPresent,
    formatPresent,
    requirementMet,
    matchedOption,
    unmetRequired,
    isReady,
    readiness,
    sortedOptions,
  }
}
