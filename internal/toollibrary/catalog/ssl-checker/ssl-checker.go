package main

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type certResult struct {
	Domain             string   `json:"domain"`
	Valid              bool     `json:"valid"`
	Issuer             string   `json:"issuer"`
	Subject            string   `json:"subject"`
	NotBefore          string   `json:"not_before"`
	NotAfter           string   `json:"not_after"`
	DaysUntilExpiry    int      `json:"days_until_expiry"`
	SANNames           []string `json:"san_names"`
	Protocol           string   `json:"protocol"`
	CipherSuite        string   `json:"cipher_suite"`
	SerialNumber       string   `json:"serial_number"`
	SignatureAlgorithm string   `json:"signature_algorithm"`
	Error              string   `json:"error,omitempty"`
}

func registerRoutes(r chi.Router) {
	r.Get("/check", handleCheck)
	r.Get("/check-multi", handleCheckMulti)
}

func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", version)
	}
}

func checkDomain(domain, port string) certResult {
	result := certResult{
		Domain:   domain,
		SANNames: []string{},
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(domain, port), &tls.Config{
		InsecureSkipVerify: false,
	})
	if err != nil {
		// Try again with InsecureSkipVerify to still get cert info for expired certs
		conn2, err2 := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(domain, port), &tls.Config{
			InsecureSkipVerify: true,
		})
		if err2 != nil {
			result.Error = fmt.Sprintf("connection failed: %v", err)
			return result
		}
		defer conn2.Close()

		state := conn2.ConnectionState()
		if len(state.PeerCertificates) == 0 {
			result.Error = "no certificates presented"
			return result
		}

		cert := state.PeerCertificates[0]

		result.Valid = false
		result.Issuer = strings.Join(cert.Issuer.Organization, ", ")
		result.Subject = cert.Subject.CommonName
		result.NotBefore = cert.NotBefore.Format(time.RFC3339)
		result.NotAfter = cert.NotAfter.Format(time.RFC3339)
		result.DaysUntilExpiry = int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))
		result.SANNames = cert.DNSNames
		result.Protocol = tlsVersionString(state.Version)
		result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
		result.SerialNumber = cert.SerialNumber.Text(16)
		result.SignatureAlgorithm = cert.SignatureAlgorithm.String()
		result.Error = fmt.Sprintf("certificate validation failed: %v", err)

		if len(result.SANNames) == 0 {
			result.SANNames = []string{}
		}

		return result
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		result.Error = "no certificates presented"
		return result
	}

	cert := state.PeerCertificates[0]
	now := time.Now()

	result.Valid = now.After(cert.NotBefore) && now.Before(cert.NotAfter)
	result.Issuer = strings.Join(cert.Issuer.Organization, ", ")
	result.Subject = cert.Subject.CommonName
	result.NotBefore = cert.NotBefore.Format(time.RFC3339)
	result.NotAfter = cert.NotAfter.Format(time.RFC3339)
	result.DaysUntilExpiry = int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))
	result.SANNames = cert.DNSNames
	result.Protocol = tlsVersionString(state.Version)
	result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
	result.SerialNumber = cert.SerialNumber.Text(16)
	result.SignatureAlgorithm = cert.SignatureAlgorithm.String()

	if len(result.SANNames) == 0 {
		result.SANNames = []string{}
	}

	return result
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain parameter is required")
		return
	}

	// Strip any protocol prefix if provided
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	// Strip any trailing path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	// Strip any port from the domain if embedded
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	}

	port := r.URL.Query().Get("port")
	if port == "" {
		port = "443"
	}

	result := checkDomain(domain, port)

	if result.Error != "" && result.Issuer == "" {
		writeError(w, http.StatusBadGateway, result.Error)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func handleCheckMulti(w http.ResponseWriter, r *http.Request) {
	domainsParam := r.URL.Query().Get("domains")
	if domainsParam == "" {
		writeError(w, http.StatusBadRequest, "domains parameter is required")
		return
	}

	domains := strings.Split(domainsParam, ",")
	// Trim whitespace from each domain
	for i := range domains {
		domains[i] = strings.TrimSpace(domains[i])
	}

	// Filter out empty strings
	var validDomains []string
	for _, d := range domains {
		if d != "" {
			validDomains = append(validDomains, d)
		}
	}

	if len(validDomains) == 0 {
		writeError(w, http.StatusBadRequest, "no valid domains provided")
		return
	}

	results := make([]certResult, len(validDomains))
	var wg sync.WaitGroup

	for i, domain := range validDomains {
		wg.Add(1)
		go func(idx int, d string) {
			defer wg.Done()

			// Clean the domain
			d = strings.TrimPrefix(d, "https://")
			d = strings.TrimPrefix(d, "http://")
			if pathIdx := strings.Index(d, "/"); pathIdx != -1 {
				d = d[:pathIdx]
			}
			if host, _, err := net.SplitHostPort(d); err == nil {
				d = host
			}

			results[idx] = checkDomain(d, "443")
		}(i, domain)
	}

	wg.Wait()

	writeJSON(w, http.StatusOK, results)
}
