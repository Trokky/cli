// Package useragent names the CLI to the instances it talks to. An instance records it with
// each sign-in, so its Studio's connected applications can tell one machine from another.
package useragent

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
)

// Version is set from cmd's build-time version.
var Version = "dev"

// String is the User-Agent: "trokky-cli/0.3.0 (macOS; amens-mbp)".
func String() string {
	return fmt.Sprintf("trokky-cli/%s (%s; %s)", Version, osName(), host())
}

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func host() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "unknown host"
	}
	return strings.TrimSuffix(name, ".local")
}

type transport struct{ base http.RoundTripper }

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", String())
	}
	return t.base.RoundTrip(req)
}

// Transport wraps base (http.DefaultTransport when nil) to send the CLI's User-Agent.
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transport{base: base}
}
