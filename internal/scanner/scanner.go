package scanner

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"
	"golang.org/x/net/publicsuffix"
)

type Result struct {
	SSLExpiry    *time.Time
	DomainExpiry *time.Time
	TLSVersion   string
	Issuer       string
	IssuerDN     string
	IPAddress    string
	Status       string
	Nameservers  []string
	Error        error
}

func ScanDomain(ctx context.Context, domain string, port int) Result {
	res := Result{}
	scanCtx, cancel := withDefaultTimeout(ctx, 12*time.Second)
	defer cancel()

	var mu sync.Mutex
	wg := sync.WaitGroup{}
	wg.Add(4)

	go func() {
		defer wg.Done()
		ip, err := resolveIP(scanCtx, domain)
		if err == nil && ip != "" {
			mu.Lock()
			res.IPAddress = ip
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		tlsCtx, cancelTLS := withDefaultTimeout(scanCtx, 8*time.Second)
		defer cancelTLS()
		tlsResult, err := fetchCertificate(tlsCtx, domain, port)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if res.Error == nil {
				res.Error = err
			}
			if res.Status == "" {
				res.Status = "No SSL"
			}
			return
		}
		res.SSLExpiry = tlsResult.SSLExpiry
		res.Issuer = tlsResult.Issuer
		res.IssuerDN = tlsResult.IssuerDN
		res.TLSVersion = tlsResult.TLSVersion
	}()

	go func() {
		defer wg.Done()
		expiry, err := resolveDomainExpiry(scanCtx, domain, 6*time.Second)
		if err == nil {
			mu.Lock()
			res.DomainExpiry = expiry
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		nsCtx, cancelNS := withDefaultTimeout(scanCtx, 4*time.Second)
		defer cancelNS()
		if ns, err := lookupNameservers(nsCtx, domain); err == nil && len(ns) > 0 {
			mu.Lock()
			res.Nameservers = ns
			mu.Unlock()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-scanCtx.Done():
	}

	if res.Status == "" {
		res.Status = deriveStatus(res.SSLExpiry)
	}
	return res
}

func deriveStatus(expiry *time.Time) string {
	if expiry == nil {
		return "Unknown"
	}
	days := int(time.Until(*expiry).Hours() / 24)
	if days < 7 {
		return "Critical"
	}
	if days < 30 {
		return "ExpiringSoon"
	}
	return "Healthy"
}

type tlsInfo struct {
	SSLExpiry  *time.Time
	Issuer     string
	IssuerDN   string
	TLSVersion string
}

func fetchCertificate(ctx context.Context, domain string, port int) (tlsInfo, error) {
	address := net.JoinHostPort(domain, intToString(port))
	dialer := &net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return tlsInfo{}, err
	}
	conn := tls.Client(rawConn, &tls.Config{
		ServerName:         domain,
		InsecureSkipVerify: true,
	})
	deadline := deadlineFromContext(ctx, 8*time.Second)
	_ = conn.SetDeadline(deadline)
	if err := conn.Handshake(); err != nil {
		_ = conn.Close()
		return tlsInfo{}, err
	}
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		_ = conn.Close()
		return tlsInfo{}, errors.New("no certificate presented")
	}

	cert := state.PeerCertificates[0]
	issuer := ""
	if len(cert.Issuer.Organization) > 0 && cert.Issuer.Organization[0] != "" {
		issuer = cert.Issuer.Organization[0]
	} else {
		issuer = cert.Issuer.CommonName
	}

	expiry := cert.NotAfter
	tlsVersion := tlsVersionName(state.Version)
	_ = conn.Close()
	return tlsInfo{
		SSLExpiry:  &expiry,
		Issuer:     issuer,
		IssuerDN:   cert.Issuer.String(),
		TLSVersion: tlsVersion,
	}, nil
}

func resolveIP(ctx context.Context, domain string) (string, error) {
	resolver := net.DefaultResolver
	ips, err := resolver.LookupIP(ctx, "ip", domain)
	if err != nil || len(ips) == 0 {
		return "", err
	}
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}
	return ips[0].String(), nil
}

func resolveDomainExpiry(ctx context.Context, domain string, timeout time.Duration) (*time.Time, error) {
	root, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		root = domain
	}

	raw, err := whoisWithTimeout(ctx, root, timeout)
	if err != nil {
		return nil, err
	}

	parsed, err := whoisparser.Parse(raw)
	if err != nil {
		return nil, err
	}

	exp := strings.TrimSpace(parsed.Domain.ExpirationDate)
	if exp == "" {
		return nil, errors.New("domain expiration not found")
	}
	parsedTime, err := parseWhoisDate(exp)
	if err != nil {
		return nil, err
	}
	return &parsedTime, nil
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return "Unknown"
	}
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

func parseWhoisDate(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05Z",
		"2006-01-02 15:04:05 MST",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05-0700",
		"02-Jan-2006",
		"02-Jan-2006 15:04:05",
		"02-Jan-2006 15:04:05 MST",
		"2006/01/02",
		"2006/01/02 15:04:05",
		"2006.01.02",
		"2006.01.02 15:04:05",
		"20060102",
	}

	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, errors.New("unsupported whois date format")
}

func lookupNameservers(ctx context.Context, domain string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "nslookup", "-type=ns", domain)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// still try to parse whatever output we got
		if len(output) == 0 {
			return nil, err
		}
	}
	nameservers := parseNameserverOutput(string(output))
	if len(nameservers) > 0 {
		return nameservers, nil
	}

	// Fallback to Go's DNS lookup for nameservers if nslookup doesn't return any.
	resolver := net.DefaultResolver
	records, lookupErr := resolver.LookupNS(ctx, domain)
	if lookupErr != nil {
		return nil, lookupErr
	}
	for _, record := range records {
		nameservers = append(nameservers, strings.TrimSuffix(strings.TrimSpace(record.Host), "."))
	}
	return uniqueStrings(nameservers), nil
}

func withDefaultTimeout(ctx context.Context, fallback time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), fallback)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, fallback)
}

func deadlineFromContext(ctx context.Context, fallback time.Duration) time.Time {
	if ctx == nil {
		return time.Now().Add(fallback)
	}
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Now().Add(fallback)
}

func whoisWithTimeout(ctx context.Context, domain string, timeout time.Duration) (string, error) {
	child, cancel := withDefaultTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		raw string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := whois.Whois(domain)
		ch <- result{raw: raw, err: err}
	}()

	select {
	case <-child.Done():
		return "", child.Err()
	case res := <-ch:
		return res.raw, res.err
	}
}

func parseNameserverOutput(output string) []string {
	var nameservers []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if !strings.Contains(lower, "nameserver") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		candidate := strings.TrimSpace(parts[1])
		candidate = strings.TrimSuffix(candidate, ".")
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		nameservers = append(nameservers, candidate)
	}
	return nameservers
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, value := range input {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
