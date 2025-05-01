package zoneedit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/libdns/libdns"
)

const (
	createURL = "https://dynamic.zoneedit.com/txt-create.php"
	deleteURL = "https://dynamic.zoneedit.com/txt-delete.php"
)

type Provider struct {
	Username string `json:"username,omitempty"`
	Token    string `json:"token,omitempty"`

	client   *http.Client
	initOnce sync.Once
}

func (p *Provider) init() error {
	var initErr error
	p.initOnce.Do(func() {

		if p.client == nil {
			p.client = &http.Client{
				Timeout: 30 * time.Second,
			}
		}
		if p.Username == "" || p.Token == "" {
			initErr = fmt.Errorf("username and token are required")
		}
	})
	return initErr

}

// AppendRecords creates each TXT record in ZoneEdit.
func (p *Provider) AppendRecords(ctx context.Context, zone string,
	recs []libdns.Record) ([]libdns.Record, error) {

	err := p.init()
	if err != nil {
		return nil, fmt.Errorf("AppendRecords: %w", err)
	}
	var created []libdns.Record

	for _, r := range recs {
		if r.RR().Type != "TXT" {
			continue // Caddy only sends TXT for ACME
		}
		if err := p.doRequest(ctx, createURL, zone, r); err != nil {
			return created, fmt.Errorf("AppendRecords: %w", err)
		}
		created = append(created, r)
	}
	return created, nil
}

// DeleteRecords removes the TXT records.
func (p *Provider) DeleteRecords(ctx context.Context, zone string,
	recs []libdns.Record) ([]libdns.Record, error) {

	err := p.init()
	if err != nil {
		return nil, fmt.Errorf("DeleteRecords: %w", err)
	}
	var deleted []libdns.Record
	for _, r := range recs {
		if r.RR().Type != "TXT" {
			continue // Caddy only sends TXT for ACME
		}
		if err := p.doRequest(ctx, deleteURL, zone, r); err != nil {
			return deleted, fmt.Errorf("DeleteRecords: %w", err)
		}
		deleted = append(deleted, r)
	}
	return deleted, nil
}

func (p *Provider) doRequest(ctx context.Context, baseURL, zone string, record libdns.Record) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}

	q := u.Query()
	q.Set("host", libdns.AbsoluteName(record.RR().Name, zone))
	q.Set("rdata", strings.Trim(record.RR().Data, `"`))
	u.RawQuery = q.Encode()
	fmt.Printf("doRequest: %s\n", u.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	req.SetBasicAuth(p.Username, p.Token)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("zoneedit request: %w", err)
	}
	fmt.Printf("doRequest: %s\n", resp)
	defer resp.Body.Close()

	// Read body to ensure connection is properly closed
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unsuccessful status code: %s - body: %s", resp.Status, string(body))
	}

	return nil
}
