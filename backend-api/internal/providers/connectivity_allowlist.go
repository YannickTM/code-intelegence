package providers

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const providerConnectivityAllowedHostsEnv = "PROVIDER_CONNECTIVITY_ALLOWED_HOSTS"

// defaultLoopbackHosts are safe defaults used when PROVIDER_CONNECTIVITY_ALLOWED_HOSTS
// is not set. They only cover loopback / host-local addresses that cannot be
// used for SSRF against external services.
var defaultLoopbackHosts = []string{
	"localhost",
	"127.0.0.1",
	"::1",
	"host.docker.internal",
}

func connectivityAllowed(rawEndpointURL string) error {
	u, err := url.Parse(rawEndpointURL)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL")
	}
	host := strings.ToLower(u.Hostname())
	hostPort := strings.ToLower(u.Host)

	// If the env var is set, use it exclusively (preserves existing behavior).
	if raw, ok := os.LookupEnv(providerConnectivityAllowedHostsEnv); ok {
		allowed := parseAllowedConnectivityHosts(raw)
		if len(allowed) == 0 {
			return fmt.Errorf("connectivity checks are not enabled for arbitrary provider endpoints")
		}
		for _, entry := range allowed {
			if entry == host || entry == hostPort {
				return nil
			}
		}
		return fmt.Errorf("connectivity checks are only allowed for configured provider test hosts")
	}

	// Env var not set: allow loopback/local addresses by default.
	if isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf(
		"connectivity checks for non-local endpoints require setting %s",
		providerConnectivityAllowedHostsEnv,
	)
}

// isLoopbackHost returns true if the hostname is a known loopback or
// host-local address. It checks the static list first, then falls back to
// DNS resolution requiring ALL resolved IPs to be loopback (prevents DNS
// rebinding attacks).
func isLoopbackHost(hostname string) bool {
	for _, h := range defaultLoopbackHosts {
		if hostname == h {
			return true
		}
	}
	ips, err := net.LookupHost(hostname)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}

func parseAllowedConnectivityHosts(raw string) []string {
	parts := strings.Split(raw, ",")
	allowed := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		allowed = append(allowed, part)
	}
	return allowed
}
