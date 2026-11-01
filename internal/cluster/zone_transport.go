package cluster

import (
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ZoneHTTPClient uses the existing cluster CA identity without sharing its
// private key or granting agent/standby authority. pin is exactly 64 hex digits.
func ZoneHTTPClient(pin string) (*http.Client, error) {
	b, err := hex.DecodeString(pin)
	if err != nil || len(b) != 32 {
		return nil, fmt.Errorf("invalid zone source fingerprint")
	}
	tr := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: AgentServerName, InsecureSkipVerify: true, VerifyConnection: pinnedVerifier("sha256:" + hex.EncodeToString(b))}, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16 << 10, MaxConnsPerHost: 1}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("zone sync redirects forbidden") }}, nil
}
