package main

import (
	"testing"

	"github.com/zamiba/forge/engine"
)

func specsWith(platforms ...[]string) []engine.Spec {
	out := make([]engine.Spec, len(platforms))
	for i, p := range platforms {
		out[i] = engine.Spec{TargetPlatforms: p}
	}
	return out
}

// The host platform gained an architecture suffix when SpaghettiKart turned out
// to ship separate mac-arm64 and mac-intel-x64 archives. Every catalog spec that
// predates that declares a bare OS name, and must keep resolving to the string
// it actually declares — the engine matches on equality, and steps compare
// against $platform with conditions like `$platform == Windows`.
func TestResolvePlatform(t *testing.T) {
	cases := []struct {
		name  string
		specs []engine.Spec
		host  string
		want  string
	}{
		{"bare spec matches any architecture of that OS",
			specsWith([]string{"Linux"}), "Linux-x64", "Linux"},
		{"bare spec on arm",
			specsWith([]string{"Linux"}), "Linux-arm64", "Linux"},
		{"architecture-qualified spec matches its own architecture",
			specsWith([]string{"Mac-arm64", "Mac-x64"}), "Mac-arm64", "Mac-arm64"},
		{"and picks the other one on intel",
			specsWith([]string{"Mac-arm64", "Mac-x64"}), "Mac-x64", "Mac-x64"},
		{"an exact match beats the bare OS regardless of order",
			specsWith([]string{"Mac", "Mac-arm64"}), "Mac-arm64", "Mac-arm64"},
		{"and beats it across separate specs",
			specsWith([]string{"Mac"}, []string{"Mac-arm64"}), "Mac-arm64", "Mac-arm64"},
		{"multi-platform spec still matches",
			specsWith([]string{"Linux", "Windows"}), "Windows-x64", "Windows"},

		// No match: fall back to the bare OS so Select returns nil and the error
		// reads "no spec for Mac" rather than "no spec for Mac-x64".
		{"an arm-only port on an intel host does not match",
			specsWith([]string{"Mac-arm64"}), "Mac-x64", "Mac"},
		{"a windows-only port on linux does not match",
			specsWith([]string{"Windows"}), "Linux-x64", "Linux"},

		// A spec declaring no platforms targets every platform, and Select
		// returns it for anything.
		{"spec with no declared platforms",
			specsWith([]string{}), "Linux-x64", "Linux"},
		{"no specs at all",
			nil, "Mac-arm64", "Mac"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolvePlatform(c.specs, c.host); got != c.want {
				t.Errorf("resolvePlatform(%v, %q) = %q, want %q",
					c.specs, c.host, got, c.want)
			}
		})
	}
}

// Select is what consumes the resolved string, so the two have to agree: the
// point of returning the spec's own spelling is that equality matching still
// finds it.
func TestResolvedPlatformSelectsTheSpec(t *testing.T) {
	specs := []engine.Spec{
		{Versions: []string{"1.0"}, TargetPlatforms: []string{"Linux"}},
		{Versions: []string{"1.0"}, TargetPlatforms: []string{"Mac-arm64"}},
		{Versions: []string{"1.0"}, TargetPlatforms: []string{"Mac-x64"}},
	}
	for host, want := range map[string]string{
		"Linux-x64":   "Linux",
		"Linux-arm64": "Linux",
		"Mac-arm64":   "Mac-arm64",
		"Mac-x64":     "Mac-x64",
	} {
		got := resolvePlatform(specs, host)
		spec := engine.Select(specs, got, "1.0")
		if spec == nil {
			t.Errorf("host %q resolved to %q, which Select did not match", host, got)
			continue
		}
		if spec.TargetPlatforms[0] != want {
			t.Errorf("host %q selected %v, want %q", host, spec.TargetPlatforms, want)
		}
	}

	// A host with no build available must select nothing, not the wrong build.
	if got := resolvePlatform(specs, "Windows-x64"); engine.Select(specs, got, "1.0") != nil {
		t.Errorf("Windows resolved to %q and matched a spec; expected no match", got)
	}
}

func TestPlatformBase(t *testing.T) {
	for in, want := range map[string]string{
		"Mac-arm64": "Mac",
		"Linux-x64": "Linux",
		"Windows":   "Windows",
		"Mac":       "Mac",
	} {
		if got := platformBase(in); got != want {
			t.Errorf("platformBase(%q) = %q, want %q", in, got, want)
		}
	}
}
