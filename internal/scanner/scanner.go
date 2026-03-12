package scanner

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"
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
	_ = ctx

	ip, err := resolveIP(domain)
	if err == nil {
		res.IPAddress = ip
	}

	tlsResult, err := fetchCertificate(ctx, domain, port)
	if err != nil {
		res.Error = err
		res.Status = "No SSL"
	} else {
		res.SSLExpiry = tlsResult.SSLExpiry
		res.Issuer = tlsResult.Issuer
		res.IssuerDN = tlsResult.IssuerDN
		res.TLSVersion = tlsResult.TLSVersion
	}

	expiry, err := resolveDomainExpiry(domain)
	if err == nil {
		res.DomainExpiry = expiry
	}

	if ns, err := lookupNameservers(ctx, domain); err == nil && len(ns) > 0 {
		res.Nameservers = ns
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
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	address := net.JoinHostPort(domain, intToString(port))
	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		ServerName:         domain,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return tlsInfo{}, err
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
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
	return tlsInfo{
		SSLExpiry:  &expiry,
		Issuer:     issuer,
		IssuerDN:   cert.Issuer.String(),
		TLSVersion: tlsVersion,
	}, nil
}

func resolveIP(domain string) (string, error) {
	ips, err := net.LookupIP(domain)
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

func resolveDomainExpiry(domain string) (*time.Time, error) {
	root, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		root = domain
	}

	raw, err := whois.Whois(root)
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
	records, lookupErr := net.LookupNS(domain)
	if lookupErr != nil {
		return nil, lookupErr
	}
	for _, record := range records {
		nameservers = append(nameservers, strings.TrimSuffix(strings.TrimSpace(record.Host), "."))
	}
	return uniqueStrings(nameservers), nil
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
