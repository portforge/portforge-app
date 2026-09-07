package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"reflect"
	"strings"
)

// runServer serves the same frontend over plain HTTP instead of putting it in a
// native window, for people whose system webview renders it badly — which in
// practice means WebKitGTK on Linux, since Windows and macOS get Chromium and
// WebKit proper.
//
// The frontend is byte-for-byte the build the desktop app ships. It is not
// modified or rebuilt for this mode: shim.js reconstructs the two globals the
// generated Wails bindings call into, so the bindings work unchanged.
func runServer(app *App, addr string, assets fs.FS) error {
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("could not open the embedded frontend: %w", err)
	}

	hub := newEventHub()
	app.events = hub.broadcast

	// startup is what the Wails lifecycle would call. The context is only used
	// for runtime calls that server mode does not make, so a plain background
	// context is enough.
	app.startup(context.Background())

	index, err := indexWithShim(dist)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/rpc/", http.StripPrefix("/rpc/", rpcHandler(app)))
	mux.Handle("/events", hub)
	mux.HandleFunc("/wails-shim.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(shimJS))
	})
	// The same artwork handler the desktop asset server mounts.
	mux.Handle("/mediaitems/", http.StripPrefix("/mediaitems/", artworkHandler(
		func() string { return app.metadataPath },
		thumbCacheDir(),
	)))
	mux.Handle("/", spaHandler(dist, index))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("could not listen on %s: %w", addr, err)
	}
	fmt.Printf("PortForge is running at http://%s\n", displayAddr(ln.Addr()))
	fmt.Println("Press Ctrl-C to stop.")

	return http.Serve(ln, localOnly(mux))
}

// displayAddr turns a listen address into one that can be pasted into a browser.
// A wildcard bind prints as localhost because 0.0.0.0 is not a destination.
func displayAddr(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

// localOnly rejects requests whose Host header names a domain, which is the
// standard defence against DNS rebinding: a page on the open web can point a
// name it controls at 127.0.0.1 and then talk to this server through it, but it
// cannot forge the Host header. Literal IPs are allowed so that binding to a LAN
// address on purpose still works.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		if host != "localhost" && net.ParseIP(host) == nil {
			http.Error(w, "requests must address this server by IP or localhost", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves the built frontend, falling back to index.html so a deep
// link or a refresh lands on the app rather than a 404.
func spaHandler(dist fs.FS, index []byte) http.Handler {
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" || clean == "index.html" {
			serveIndex(w, index)
			return
		}
		if _, err := fs.Stat(dist, clean); err != nil {
			serveIndex(w, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(index)
}

// indexWithShim injects the shim ahead of everything else in the document. It
// has to run before the app bundle, because the generated bindings read
// window.go at call time and the module preload can start immediately.
func indexWithShim(dist fs.FS) ([]byte, error) {
	raw, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil, fmt.Errorf("could not read the embedded index.html: %w", err)
	}
	const tag = `<script src="/wails-shim.js"></script>`
	html := string(raw)
	i := strings.Index(html, "<head>")
	if i < 0 {
		return nil, fmt.Errorf("the embedded index.html has no <head> to inject the bridge into")
	}
	i += len("<head>")
	return []byte(html[:i] + "\n    " + tag + html[i:]), nil
}

// rpcRequest is the JSON body of a call: the positional arguments the binding
// was given, exactly as the generated wrapper passes them.
type rpcResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// rpcHandler dispatches POST /rpc/<Method> onto App by reflection. Using
// reflection rather than a generated table is deliberate: Wails binds every
// exported method on App, so anything else would be a second list to keep in
// step, and a method added for the desktop build would be missing here.
func rpcHandler(app *App) http.Handler {
	target := reflect.ValueOf(app)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.Trim(r.URL.Path, "/")
		method := target.MethodByName(name)
		// MethodByName only ever finds exported methods, so unexported helpers
		// are not reachable from the browser.
		if !method.IsValid() {
			writeRPCError(w, http.StatusNotFound, fmt.Sprintf("no such method %q", name))
			return
		}

		var raw []json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeRPCError(w, http.StatusBadRequest, fmt.Sprintf("could not read the arguments for %s: %v", name, err))
			return
		}

		args, err := decodeArgs(method.Type(), raw)
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, fmt.Sprintf("%s: %v", name, err))
			return
		}

		out := method.Call(args)
		result, callErr := splitResults(out)
		if callErr != nil {
			// A method returning an error is reporting a normal failure the UI
			// knows how to show, not a broken request, so this is still a 200
			// with the error in the body — the shim rejects the promise either way.
			writeJSON(w, http.StatusOK, rpcResponse{Error: callErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rpcResponse{Result: result})
	})
}

// decodeArgs unmarshals the JSON arguments into the method's parameter types.
func decodeArgs(mt reflect.Type, raw []json.RawMessage) ([]reflect.Value, error) {
	if mt.IsVariadic() {
		return nil, fmt.Errorf("variadic methods are not callable over RPC")
	}
	if len(raw) != mt.NumIn() {
		return nil, fmt.Errorf("expected %d argument(s), got %d", mt.NumIn(), len(raw))
	}
	args := make([]reflect.Value, mt.NumIn())
	for i := 0; i < mt.NumIn(); i++ {
		v := reflect.New(mt.In(i))
		if err := json.Unmarshal(raw[i], v.Interface()); err != nil {
			return nil, fmt.Errorf("argument %d: %w", i+1, err)
		}
		args[i] = v.Elem()
	}
	return args, nil
}

// splitResults separates a trailing error return from the value, matching how
// Wails presents a bound method to JavaScript.
func splitResults(out []reflect.Value) (interface{}, error) {
	var result interface{}
	var err error
	for i, v := range out {
		if i == len(out)-1 && v.Type() == reflect.TypeOf((*error)(nil)).Elem() {
			if !v.IsNil() {
				err = v.Interface().(error)
			}
			continue
		}
		result = v.Interface()
	}
	return result, err
}

func writeJSON(w http.ResponseWriter, status int, body rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeRPCError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, rpcResponse{Error: msg})
}
