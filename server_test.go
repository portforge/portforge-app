package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// post calls one bound method the way the shim does.
func post(t *testing.T, h http.Handler, method, body string) (int, rpcResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/"+method, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out rpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: response was not JSON: %v (%s)", method, err, rec.Body.String())
	}
	return rec.Code, out
}

func TestRPCDispatchesBoundMethods(t *testing.T) {
	h := rpcHandler(NewApp())

	code, res := post(t, h, "GetPlatform", "[]")
	if code != http.StatusOK || res.Error != "" {
		t.Fatalf("GetPlatform: got %d %q", code, res.Error)
	}
	// The platform string is what install specs match on, so an empty one would
	// mean every build silently fails to resolve.
	if s, _ := res.Result.(string); s == "" {
		t.Fatalf("GetPlatform returned no platform: %#v", res.Result)
	}
}

func TestRPCRejectsBadCalls(t *testing.T) {
	h := rpcHandler(NewApp())

	if code, res := post(t, h, "NoSuchMethod", "[]"); code != http.StatusNotFound || res.Error == "" {
		t.Errorf("unknown method: got %d %q, want 404 with an error", code, res.Error)
	}
	if code, res := post(t, h, "GetPlatform", "[1,2]"); code != http.StatusBadRequest || res.Error == "" {
		t.Errorf("wrong arity: got %d %q, want 400 with an error", code, res.Error)
	}
	if code, res := post(t, h, "GetPlatform", "not json"); code != http.StatusBadRequest || res.Error == "" {
		t.Errorf("malformed body: got %d %q, want 400 with an error", code, res.Error)
	}

	// Only exported methods are reachable: reflect cannot address the rest, and
	// the browser must not be able to drive internals like startup.
	if code, _ := post(t, h, "startup", "[]"); code != http.StatusNotFound {
		t.Errorf("unexported method was reachable: got %d, want 404", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/GetPlatform", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on an RPC path: got %d, want 405", rec.Code)
	}
}

// A method's error return is a normal outcome the UI renders, not a transport
// failure, so it comes back as 200 with the message in the body.
func TestRPCReturnsMethodErrorInBody(t *testing.T) {
	app := NewApp()
	app.events = func(string, interface{}) {} // no native window, as under -server
	code, res := post(t, rpcHandler(app), "AddStorageUnit", "[]")
	if code != http.StatusOK {
		t.Fatalf("got %d, want 200", code)
	}
	if res.Error == "" {
		t.Fatal("a method that failed reported no error")
	}
}

func TestEmitPrefersTheAttachedFrontend(t *testing.T) {
	var gotName string
	var gotData interface{}
	app := NewApp()
	app.events = func(name string, data interface{}) { gotName, gotData = name, data }

	app.emit("install:progress", map[string]interface{}{"percent": 42})

	if gotName != "install:progress" {
		t.Errorf("event name = %q", gotName)
	}
	if m, ok := gotData.(map[string]interface{}); !ok || m["percent"] != 42 {
		t.Errorf("event payload = %#v", gotData)
	}
}

func TestEventHubStreamsToConnectedClients(t *testing.T) {
	hub := newEventHub()
	srv := httptest.NewServer(hub)
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	// The client registers after its request is served, so keep emitting until
	// one gets through rather than racing the subscription.
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			hub.broadcast("install:step", map[string]interface{}{"label": "Fetching"})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	scanner := bufio.NewScanner(res.Body)
	deadline := time.Now().Add(5 * time.Second)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			if time.Now().After(deadline) {
				t.Fatal("no event arrived within the deadline")
			}
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("event was not JSON: %v (%s)", err, line)
		}
		if ev.Name != "install:step" {
			t.Fatalf("event name = %q", ev.Name)
		}
		// The payload is a list because the shim spreads it into the callback,
		// matching what Wails' EventsEmit delivers.
		if len(ev.Data) != 1 {
			t.Fatalf("payload = %#v, want exactly one argument", ev.Data)
		}
		return
	}
	t.Fatal("the stream closed before delivering an event")
}

// A full queue must not stall the build that is producing the events.
func TestBroadcastDoesNotBlockOnAStalledClient(t *testing.T) {
	hub := newEventHub()
	hub.add() // never read from

	done := make(chan struct{})
	go func() {
		for i := 0; i < eventQueueDepth*2; i++ {
			hub.broadcast("install:log", map[string]interface{}{"line": "x"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast blocked on a client that stopped reading")
	}
}

func TestIndexGetsTheShimBeforeTheAppBundle(t *testing.T) {
	dist := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<!DOCTYPE html><html><head><script type="module" src="/assets/index.js"></script></head><body></body></html>`)},
	}
	out, err := indexWithShim(dist)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	shim := strings.Index(html, "/wails-shim.js")
	bundle := strings.Index(html, "/assets/index.js")
	if shim < 0 {
		t.Fatal("the shim was not injected")
	}
	// The generated bindings read window.go at call time, so the shim has to be
	// evaluated before the bundle that calls them.
	if shim > bundle {
		t.Fatal("the shim was injected after the app bundle")
	}
}

func TestIndexWithShimReportsAMissingHead(t *testing.T) {
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(`<html><body></body></html>`)}}
	if _, err := indexWithShim(dist); err == nil {
		t.Fatal("expected an error when there is nowhere to inject the shim")
	}
}

// DNS rebinding: a page on the open web points a name it controls at 127.0.0.1
// and talks to this server through it. It cannot forge the Host header, so the
// Host is what gives it away.
func TestLocalOnlyRejectsDomainHosts(t *testing.T) {
	h := localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	allowed := []string{"localhost:34116", "127.0.0.1:34116", "192.168.1.10:34116", "[::1]:34116"}
	for _, host := range allowed {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("host %q: got %d, want 200", host, rec.Code)
		}
	}

	blocked := []string{"evil.example.com:34116", "portforge.local:34116", "evil.example.com"}
	for _, host := range blocked {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("host %q: got %d, want 403", host, rec.Code)
		}
	}
}

func TestSPAFallsBackToIndex(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html><head></head></html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	h := spaHandler(dist, []byte("INDEX"))

	for path, want := range map[string]string{
		"/":              "INDEX",
		"/games/some-id": "INDEX", // a refresh on a deep link must not 404
		"/assets/app.js": "console.log(1)",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Body.String(); got != want {
			t.Errorf("GET %s = %q, want %q", path, got, want)
		}
	}
}
