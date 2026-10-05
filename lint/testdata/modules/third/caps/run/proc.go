package run

import (
	"net/http/httputil"
	"net/netip"
	"net/rpc"
	"net/url"
	"os"
)

// Spawn starts the program at path.
func Spawn(path string) (*os.Process, error) {
	return os.StartProcess(path, []string{path}, &os.ProcAttr{})
}

// Stop ends the process with the id pid.
func Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// Proxy forwards requests to target.
func Proxy(target *url.URL) *httputil.ReverseProxy { return httputil.NewSingleHostReverseProxy(target) }

// Dial connects to the RPC server at addr.
func Dial(addr netip.AddrPort) (*rpc.Client, error) { return rpc.Dial("tcp", addr.String()) }

// Home returns the home folder, which os allows.
func Home() (string, error) { return os.UserHomeDir() }
