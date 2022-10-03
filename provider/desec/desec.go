package desec

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
	"strings"
)

const (
	defaultDesecRecordTTL = 3600
)

type DesecProvider struct {
	provider.BaseProvider
	// only consider hosted zones managing domains ending in this suffix
	domainFilter endpoint.DomainFilter
	DryRun       bool
	client       *API
}

func NewDesecProvider(ctx context.Context, domainFilter endpoint.DomainFilter, dryRun bool, apiPageSize int) (*DesecProvider, error) {
	token, ok := os.LookupEnv("DESEC_API_TOKEN")
	if !ok {
		return nil, fmt.Errorf("no token found")
	}

	p := &DesecProvider{
		domainFilter: domainFilter,
		DryRun:       dryRun,
		client:       &API{Token: token},
	}

	return p, nil
}

func (p *DesecProvider) Domains(ctx context.Context) ([]DNSDomain, error) {
	domains, err := p.client.GetDNSDomains(ctx)
	if err != nil {
		return nil, err
	}

	var result []DNSDomain
	for _, domain := range domains {
		if p.domainFilter.Match(domain.Name) {
			result = append(result, domain)
		}
	}

	return result, nil
}

func (p *DesecProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	domains, err := p.Domains(ctx)
	if err != nil {
		return nil, err
	}

	var endpoints []*endpoint.Endpoint
	for _, domain := range domains {
		rrsets, err := p.client.GetAllRRSets(domain.Name)
		if err != nil {
			return nil, err
		}
		for _, rrset := range rrsets {
			if !provider.SupportedRecordType(rrset.Type) {
				continue
			}
			ep := endpoint.NewEndpointWithTTL(rrset.Name, rrset.Type, endpoint.TTL(rrset.TTL), rrset.Records...)
			endpoints = append(endpoints, ep)
		}
	}

	log.WithFields(log.Fields{
		"endpoints": endpoints,
	}).Debug("Endpoints generated from desec.io DNS")

	return endpoints, nil
}

func (p *DesecProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	log.WithFields(log.Fields{
		"changes": changes,
	}).Debug("Changes for desec.io DNS")

	domains, err := p.client.GetDNSDomains(nil)
	if err != nil {
		return err
	}
	bulk := make(map[string]RRSets)
	for _, d := range domains {
		bulk[d.Name] = RRSets{}
	}

	addToBulk(bulk, changes.Create...)
	addToBulk(bulk, changes.UpdateNew...)
	for _, change := range changes.Delete {
		change.Targets = endpoint.Targets{}
		addToBulk(bulk, change)
	}

	for domain, rrsets := range bulk {
		if len(rrsets) == 0 {
			continue
		}
		err := p.client.BulkUpdateRRSet(domain, rrsets)
		if err != nil {
			return err
		}
	}

	return nil
}

func addToBulk(bulk map[string]RRSets, changes ...*endpoint.Endpoint) {
change:
	for _, change := range changes {
		for domain, _ := range bulk {
			if !strings.HasSuffix(change.DNSName, domain) {
				continue
			}

			ttl := defaultDesecRecordTTL
			if change.RecordTTL.IsConfigured() {
				ttl = int(change.RecordTTL)
			}

			rrset := &RRSet{
				Domain:  domain,
				SubName: change.DNSName[:strings.Index(change.DNSName, domain)-1],
				Name:    change.DNSName,
				Type:    change.RecordType,
				Records: []string{},
				TTL:     ttl,
			}

			for _, target := range change.Targets {
				rrset.Records = append(rrset.Records, target)
			}
			bulk[domain] = append(bulk[domain], rrset)
			continue change
		}
		log.Debugf("Skipping change %v because no matching domain was found", change)
	}
}
