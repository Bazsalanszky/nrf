// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/openapi/v2/models"
)

// TestInitNrfContextAddresses checks the addresses of the NRF's own profile
// and services: the IpEndPoint family follows the configured address (one
// endpoint per family, IPv4 first), a host name goes into fqdn with a
// port-only endpoint, and the ApiPrefix brackets an IPv6 host.
func TestInitNrfContextAddresses(t *testing.T) {
	const (
		v4   = "10.42.0.30"
		v6   = "2001:db8:600::30"
		fqdn = "nrf"
	)
	tests := []struct {
		name          string
		sbi           factory.Sbi
		env           map[string]string
		wantEndpoints string // JSON of every service's ipEndPoints
		wantFqdn      string // profile and service fqdn
		wantIPv4      []string
		wantIPv6      []string
		wantPrefix    string
	}{
		{
			name:          "IPv4",
			sbi:           factory.Sbi{RegisterIPv4: v4},
			wantEndpoints: `[{"ipv4Address":"10.42.0.30","port":8000,"transport":"TCP"}]`,
			wantIPv4:      []string{v4},
			wantPrefix:    "http://10.42.0.30:8000",
		},
		{
			name:          "host name",
			sbi:           factory.Sbi{RegisterIPv4: fqdn},
			wantEndpoints: `[{"port":8000,"transport":"TCP"}]`,
			wantFqdn:      fqdn,
			wantPrefix:    "http://nrf:8000",
		},
		{
			name:          "IPv6 under registerIPv4",
			sbi:           factory.Sbi{RegisterIPv4: v6},
			wantEndpoints: `[{"ipv6Address":"2001:db8:600::30","port":8000,"transport":"TCP"}]`,
			wantIPv6:      []string{v6},
			wantPrefix:    "http://[2001:db8:600::30]:8000",
		},
		{
			name:          "dual stack from POD_IPS",
			sbi:           factory.Sbi{RegisterIPv4: "POD_IPS"},
			env:           map[string]string{"POD_IPS": v6 + "," + v4},
			wantEndpoints: `[{"ipv4Address":"10.42.0.30","port":8000,"transport":"TCP"},{"ipv6Address":"2001:db8:600::30","port":8000,"transport":"TCP"}]`,
			wantIPv4:      []string{v4},
			wantIPv6:      []string{v6},
			wantPrefix:    "http://10.42.0.30:8000",
		},
		{
			name:          "dual stack, preferredFamily IPv6",
			sbi:           factory.Sbi{RegisterIPv4: v4, RegisterIPv6: v6, PreferredFamily: "IPv6"},
			wantEndpoints: `[{"ipv4Address":"10.42.0.30","port":8000,"transport":"TCP"},{"ipv6Address":"2001:db8:600::30","port":8000,"transport":"TCP"}]`,
			wantIPv4:      []string{v4},
			wantIPv6:      []string{v6},
			wantPrefix:    "http://[2001:db8:600::30]:8000",
		},
	}

	previous := factory.NrfConfig
	previousProfile := NrfNfProfile
	t.Cleanup(func() {
		factory.NrfConfig = previous
		NrfNfProfile = previousProfile
	})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			sbi := tc.sbi
			sbi.Scheme, sbi.Port = "http", 8000
			factory.NrfConfig = factory.Config{
				Info: &factory.Info{Version: "1.0.0"},
				Configuration: &factory.Configuration{
					Sbi:             &sbi,
					ServiceNameList: []string{"nnrf-nfm", "nnrf-disc"},
				},
			}
			NrfNfProfile = models.NFProfile{}
			InitNrfContext()

			if got := NrfNfProfile.GetFqdn(); got != tc.wantFqdn {
				t.Errorf("profile fqdn = %q, want %q", got, tc.wantFqdn)
			}
			if got := NrfNfProfile.GetIpv4Addresses(); !slices.Equal(got, tc.wantIPv4) {
				t.Errorf("profile ipv4Addresses = %v, want %v", got, tc.wantIPv4)
			}
			if got := NrfNfProfile.GetIpv6Addresses(); !slices.Equal(got, tc.wantIPv6) {
				t.Errorf("profile ipv6Addresses = %v, want %v", got, tc.wantIPv6)
			}
			services := NrfNfProfile.GetNfServices()
			if len(services) != 2 || len(NrfNfProfile.GetNfServiceList()) != 2 {
				t.Fatalf("got %d nfServices and %d nfServiceList entries, want 2 each",
					len(services), len(NrfNfProfile.GetNfServiceList()))
			}
			for _, svc := range services {
				endpoints, err := json.Marshal(svc.GetIpEndPoints())
				if err != nil {
					t.Fatal(err)
				}
				if string(endpoints) != tc.wantEndpoints {
					t.Errorf("%s ipEndPoints = %s, want %s", svc.GetServiceName(), endpoints, tc.wantEndpoints)
				}
				if got := svc.GetFqdn(); got != tc.wantFqdn {
					t.Errorf("%s fqdn = %q, want %q", svc.GetServiceName(), got, tc.wantFqdn)
				}
				if got := svc.GetApiPrefix(); got != tc.wantPrefix {
					t.Errorf("%s apiPrefix = %q, want %q", svc.GetServiceName(), got, tc.wantPrefix)
				}
			}
		})
	}
}
