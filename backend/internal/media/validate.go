package media

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

var DefaultResolver Resolver = net.DefaultResolver

func Validate(raw string) error {
	_, err := NormalizeURL(raw, allowedHostsFromEnv())
	return err
}

func NormalizeURL(raw string, allowedHosts []string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	u, err := url.ParseRequestURI(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("invalid source URL")
	}
	if u.User != nil {
		return "", fmt.Errorf("credentials in source URL are not allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", fmt.Errorf("private source address is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && private(ip) {
		return "", fmt.Errorf("private source address is not allowed")
	}
	if len(allowedHosts) > 0 && !hostAllowed(host, allowedHosts) {
		return "", fmt.Errorf("source host %q is not in allowlist", host)
	}
	u.Fragment = ""
	return u.String(), nil
}

func ValidateResolvedHost(ctx context.Context, raw string, resolver Resolver) error {
	if err := Validate(raw); err != nil {
		return err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid source URL")
	}
	if resolver == nil {
		resolver = DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("source host cannot be resolved")
	}
	for _, address := range addresses {
		if private(address.IP) {
			return fmt.Errorf("private source address is not allowed")
		}
	}
	return nil
}

func SafeRedirectPolicy(ctx context.Context, resolver Resolver, allowedHosts []string) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		normalized, err := NormalizeURL(req.URL.String(), allowedHosts)
		if err != nil {
			return err
		}
		return ValidateResolvedHost(ctx, normalized, resolver)
	}
}

func hostAllowed(host string, allowed []string) bool {
	for _, entry := range allowed {
		candidate := strings.ToLower(strings.TrimSpace(entry))
		if candidate == "" {
			continue
		}
		if host == candidate || strings.HasSuffix(host, "."+candidate) {
			return true
		}
	}
	return false
}

func allowedHostsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_SOURCE_HOSTS"))
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if h := strings.ToLower(strings.TrimSpace(p)); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func private(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}
