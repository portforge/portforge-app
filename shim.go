package main

// shimJS reconstructs the two globals the Wails runtime injects into the
// webview, so the generated bindings under frontend/wailsjs work untouched:
// they call window.go.main.App.<Method>(...) and window.runtime.<fn>(...) and
// neither knows or cares what is behind them.
//
// This is why server mode needs no second frontend build. Anything the desktop
// build can call, the browser can call, including methods added later.
const shimJS = `
(function () {
  'use strict'

  function call(name, args) {
    return fetch('/rpc/' + encodeURIComponent(name), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(args),
    }).then(function (res) {
      return res.json().catch(function () { return null }).then(function (body) {
        if (body && body.error) throw new Error(body.error)
        if (!res.ok) throw new Error('PortForge server: ' + res.status + ' ' + res.statusText)
        return body ? body.result : undefined
      })
    })
  }

  // A Proxy stands in for the generated binding object so the server never has
  // to publish a method list: any property read becomes a call of that name,
  // which is exactly what the bindings expect and what reflection dispatches.
  var app = new Proxy({}, {
    get: function (_target, prop) {
      if (typeof prop !== 'string') return undefined
      return function () {
        return call(prop, Array.prototype.slice.call(arguments))
      }
    },
  })
  window.go = { main: { App: app } }

  // name -> Set of {cb, max}. max counts down for EventsOnce; -1 is forever.
  var listeners = new Map()

  function on(name, cb, max) {
    var subs = listeners.get(name)
    if (!subs) { subs = new Set(); listeners.set(name, subs) }
    var sub = { cb: cb, max: typeof max === 'number' ? max : -1 }
    subs.add(sub)
    return function () { subs.delete(sub) }
  }

  function dispatch(name, data) {
    var subs = listeners.get(name)
    if (!subs) return
    // Copy first: a callback may unsubscribe itself while we are iterating.
    Array.prototype.forEach.call(Array.from(subs), function (sub) {
      try {
        sub.cb.apply(null, data || [])
      } catch (e) {
        console.error('PortForge: listener for "' + name + '" threw', e)
      }
      if (sub.max > 0) {
        sub.max -= 1
        if (sub.max === 0) subs.delete(sub)
      }
    })
  }

  var source = new EventSource('/events')
  source.onmessage = function (ev) {
    var msg
    try { msg = JSON.parse(ev.data) } catch (e) { return }
    if (msg && msg.name) dispatch(msg.name, msg.data)
  }
  // EventSource reconnects on its own; this only surfaces the gap in the console
  // so a stopped server does not look like a frozen UI.
  source.onerror = function () {
    if (source.readyState === EventSource.CLOSED) {
      console.warn('PortForge: event stream closed — is the server still running?')
    }
  }

  window.runtime = {
    EventsOn: function (name, cb) { return on(name, cb, -1) },
    EventsOnMultiple: function (name, cb, max) { return on(name, cb, max) },
    EventsOnce: function (name, cb) { return on(name, cb, 1) },
    EventsOff: function (name) {
      listeners.delete(name)
      for (var i = 1; i < arguments.length; i++) listeners.delete(arguments[i])
    },
    EventsOffAll: function () { listeners.clear() },
    // Frontend-emitted events stay in the frontend. The desktop runtime would
    // also deliver them to Go, but no Go code subscribes, so there is nothing
    // on the other side to reach.
    EventsEmit: function (name) {
      dispatch(name, Array.prototype.slice.call(arguments, 1))
    },

    LogPrint: console.log.bind(console),
    LogTrace: console.debug.bind(console),
    LogDebug: console.debug.bind(console),
    LogInfo: console.info.bind(console),
    LogWarning: console.warn.bind(console),
    LogError: console.error.bind(console),
    LogFatal: console.error.bind(console),

    BrowserOpenURL: function (url) { window.open(url, '_blank', 'noopener') },
    WindowReload: function () { window.location.reload() },
    WindowReloadApp: function () { window.location.reload() },
  }
})()
`
