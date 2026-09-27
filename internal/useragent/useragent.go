// Package useragent names the CLI to the instances it talks to: "trokky-cli/0.3.0" on every
// request, and on a sign-in, which the instance records so its Studio's connected
// applications can tell one machine from another, "trokky-cli/0.3.0 (macOS; amens-mbp)". The
// machine name goes only there, not into every request a proxy logs.
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

// String is the User-Agent of an ordinary request.
func String() string {
	return "trokky-cli/" + Version
}

// SignIn is the User-Agent of a sign-in, naming the machine.
func SignIn() string {
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
	return clean(strings.TrimSuffix(name, ".local"))
}

// clean keeps a value printable ASCII, without the comment syntax, as a header value should be
func clean(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r < 0x20 || r > 0x7e || r == '(' || r == ')' || r == ';' {
			b.WriteByte('?')
		} else {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 60 {
		out = out[:60]
	}
	if out == "" {
		return "unknown host"
	}
	return out
}

type transport struct {
	base  http.RoundTripper
	agent func() string
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.agent())
	}
	return t.base.RoundTrip(req)
}

// Transport wraps base (http.DefaultTransport when nil) to send the CLI's User-Agent.
func Transport(base http.RoundTripper) http.RoundTripper {
	return wrap(base, String)
}

// SignInTransport is Transport for the requests of a sign-in, naming the machine.
func SignInTransport(base http.RoundTripper) http.RoundTripper {
	return wrap(base, SignIn)
}

func wrap(base http.RoundTripper, agent func() string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transport{base: base, agent: agent}
}
