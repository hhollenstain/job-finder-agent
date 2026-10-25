package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const userAgent = "JobFinderAgent/0.1 (personal job search)"

func newClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return CheckURL(req.URL.String())
		},
	}
}

var httpClient = newClient()

// CheckURL rejects non-HTTP URLs and link-local metadata hosts.
func CheckURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must be http or https")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("url is missing a host")
	}
	if host == "metadata.google.internal" || host == "169.254.169.254" {
		return fmt.Errorf("refusing metadata host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.Equal(net.ParseIP("169.254.169.254")) {
			return fmt.Errorf("refusing link-local address")
		}
	}
	return nil
}

func getBytes(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	if err := CheckURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, application/xml, text/xml, */*")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 180 {
			snippet = snippet[:180]
		}
		return nil, fmt.Errorf("source returned %s: %s", res.Status, snippet)
	}
	return body, nil
}

func getJSON(ctx context.Context, rawURL string, headers map[string]string, dest any) error {
	body, err := getBytes(ctx, rawURL, headers)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}
