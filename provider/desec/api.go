package desec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://desec.io/api/v1"

// API is the basic implemenataion of an API client for desec.io
type API struct {
	Token string
}

// ErrorResponse defines the error response format
type ErrorResponse struct {
	Detail string `json:"detail,omitempty"`
}

// DNSDomain defines the format of a Domain object
type DNSDomain struct {
	Created    string `json:"created,omitempty"`
	Published  string `json:"published,omitempty"`
	Name       string `json:"name,omitempty"`
	MinimumTTL int    `json:"minimum_ttl,omitempty"`
	Touched    string `json:"touched,omitempty"`
}

// DNSDomains is a slice of Domain objects
type DNSDomains []DNSDomain

// RRSet defines the format of a Resource Record Set object
type RRSet struct {
	Domain  string   `json:"domain,omitempty"`
	SubName string   `json:"subname,omitempty"`
	Name    string   `json:"name,omitempty"`
	Type    string   `json:"type,omitempty"`
	Records []string `json:"records"`
	TTL     int      `json:"ttl,omitempty"`
	Created string   `json:"created,omitempty"`
	Touched string   `json:"touched,omitempty"`
}

// RRSets is a slice of Resource Record Set objects
type RRSets []*RRSet

// Request builds the raw request
func (a *API) request(ctx context.Context, method, path string, body io.Reader, target interface{}) error {
	if path[0] != '/' {
		path = "/" + path
	}

	url := baseURL + path

	client := &http.Client{Timeout: time.Second * 10}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+a.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	//if resp.StatusCode != http.StatusOK {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("%s %s unknown error occured", method, path)
		}
		return fmt.Errorf("%s %s error: %q", method, path, string(b))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("%s %s response parsing error: %v", method, path, err)
	}

	return nil
}

// GetDNSDomains - returns all dns domains managed by deSEC
func (a *API) GetDNSDomains(ctx context.Context) (DNSDomains, error) {
	method, path := "GET", "domains/"
	domains := new(DNSDomains)
	err := a.request(ctx, method, path, nil, domains)
	if err != nil {
		return nil, err
	}
	return *domains, nil
}

func (a *API) GetAllRRSets(domainName string) (RRSets, error) {
	method := "GET"
	path := "domains/" + domainName + "/rrsets/"

	rrsets := new(RRSets)
	err := a.request(nil, method, path, nil, rrsets)
	if err != nil {
		return nil, err
	}
	return *rrsets, nil
}

func (a *API) BulkUpdateRRSet(domainName string, rrsets RRSets) error {
	rawJSON, err := json.Marshal(rrsets)
	if err != nil {
		return err
	}
	//fmt.Printf("rawJSON = %s\n", string(rawJSON)) // debug
	method := "PUT"
	path := "domains/" + domainName + "/rrsets/"
	err = a.request(nil, method, path, bytes.NewBuffer(rawJSON), &rrsets)
	if err != nil {
		return err
	}
	return nil
}
