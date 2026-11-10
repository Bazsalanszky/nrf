// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/omec-project/util/sbiaddr"
)

const (
	testRegIPv4 = "10.42.0.20"
	testRegIPv6 = "2001:db8:600::20"
	testRegFQDN = "nrf"
	envPodIP    = "POD_IP"
	envPodIPs   = "POD_IPS"

	addrDefault = "127.0.0.1:29510"
	addrV4      = "10.42.0.20:29510"
	uriV4       = "http://10.42.0.20:29510"
	addrV6      = "[2001:db8:600::20]:29510"
	uriV6       = "http://[2001:db8:600::20]:29510"
	addrFQDN    = "nrf:29510"
	uriFQDN     = "http://nrf:29510"
)

func registerConfig(sbi Sbi) Config {
	if sbi.Scheme == "" {
		sbi.Scheme = "http"
	}
	return Config{Configuration: &Configuration{Sbi: &sbi}}
}

func TestGetSbiRegisterAddrs(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		env          map[string]string
		want         sbiaddr.RegisterAddrs
		wantHost     string // GetSbiRegisterIP
		wantAddr     string // GetSbiRegisterAddr
		wantURI      string // GetSbiUri
		wantWarnings bool
	}{
		{
			name:     "no configuration",
			cfg:      Config{},
			want:     sbiaddr.RegisterAddrs{IPv4: NRF_DEFAULT_IPV4},
			wantHost: NRF_DEFAULT_IPV4,
			wantAddr: addrDefault,
			wantURI:  "https://127.0.0.1:29510",
		},
		{
			// GetSbiRegisterAddr used to dereference the missing sbi section.
			name:     "no sbi section",
			cfg:      Config{Configuration: &Configuration{}},
			want:     sbiaddr.RegisterAddrs{IPv4: NRF_DEFAULT_IPV4},
			wantHost: NRF_DEFAULT_IPV4,
			wantAddr: addrDefault,
			wantURI:  "https://127.0.0.1:29510",
		},
		{
			name:     "IPv4",
			cfg:      registerConfig(Sbi{RegisterIPv4: testRegIPv4, Port: 8000}),
			want:     sbiaddr.RegisterAddrs{IPv4: testRegIPv4},
			wantHost: testRegIPv4,
			wantAddr: "10.42.0.20:8000",
			wantURI:  "http://10.42.0.20:8000",
		},
		{
			name:     "host name (the chart default)",
			cfg:      registerConfig(Sbi{RegisterIPv4: testRegFQDN}),
			want:     sbiaddr.RegisterAddrs{FQDN: testRegFQDN},
			wantHost: testRegFQDN,
			wantAddr: addrFQDN,
			wantURI:  uriFQDN,
		},
		{
			name:     "IPv6 under registerIPv4",
			cfg:      registerConfig(Sbi{RegisterIPv4: testRegIPv6}),
			want:     sbiaddr.RegisterAddrs{IPv6: testRegIPv6},
			wantHost: testRegIPv6,
			wantAddr: addrV6,
			wantURI:  uriV6,
		},
		{
			name:     "IPv6 under registerIPv6, in non-canonical form",
			cfg:      registerConfig(Sbi{RegisterIPv6: "[2001:DB8:600:0::20]"}),
			want:     sbiaddr.RegisterAddrs{IPv6: testRegIPv6},
			wantHost: testRegIPv6,
			wantAddr: addrV6,
			wantURI:  uriV6,
		},
		{
			name:     "dual stack, IPv4 preferred by default",
			cfg:      registerConfig(Sbi{RegisterIPv4: testRegIPv4, RegisterIPv6: testRegIPv6}),
			want:     sbiaddr.RegisterAddrs{IPv4: testRegIPv4, IPv6: testRegIPv6},
			wantHost: testRegIPv4,
			wantAddr: addrV4,
			wantURI:  uriV4,
		},
		{
			name:     "dual stack, preferredFamily IPv6",
			cfg:      registerConfig(Sbi{RegisterIPv4: testRegIPv4, RegisterIPv6: testRegIPv6, PreferredFamily: "IPv6"}),
			want:     sbiaddr.RegisterAddrs{IPv4: testRegIPv4, IPv6: testRegIPv6},
			wantHost: testRegIPv6,
			wantAddr: addrV6,
			wantURI:  uriV6,
		},
		{
			name:     "host name and IPv6: the name is used in URIs",
			cfg:      registerConfig(Sbi{RegisterIPv4: testRegFQDN, RegisterIPv6: testRegIPv6, PreferredFamily: "IPv6"}),
			want:     sbiaddr.RegisterAddrs{FQDN: testRegFQDN, IPv6: testRegIPv6},
			wantHost: testRegFQDN,
			wantAddr: addrFQDN,
			wantURI:  uriFQDN,
		},
		{
			name:         "host name and IPv4: the IPv4 address is dropped",
			cfg:          registerConfig(Sbi{RegisterIPv4: testRegFQDN + "," + testRegIPv4}),
			want:         sbiaddr.RegisterAddrs{FQDN: testRegFQDN},
			wantHost:     testRegFQDN,
			wantAddr:     addrFQDN,
			wantURI:      uriFQDN,
			wantWarnings: true,
		},
		{
			name:     "POD_IP holding IPv6 under registerIPv4",
			cfg:      registerConfig(Sbi{RegisterIPv4: envPodIP}),
			env:      map[string]string{envPodIP: testRegIPv6},
			want:     sbiaddr.RegisterAddrs{IPv6: testRegIPv6},
			wantHost: testRegIPv6,
			wantAddr: addrV6,
			wantURI:  uriV6,
		},
		{
			name:     "POD_IP holding IPv4",
			cfg:      registerConfig(Sbi{RegisterIPv4: envPodIP}),
			env:      map[string]string{envPodIP: testRegIPv4},
			want:     sbiaddr.RegisterAddrs{IPv4: testRegIPv4},
			wantHost: testRegIPv4,
			wantAddr: addrV4,
			wantURI:  uriV4,
		},
		{
			name:     "POD_IPS dual stack, preferredFamily IPv6",
			cfg:      registerConfig(Sbi{RegisterIPv4: envPodIPs, PreferredFamily: "ipv6"}),
			env:      map[string]string{envPodIPs: testRegIPv4 + "," + testRegIPv6},
			want:     sbiaddr.RegisterAddrs{IPv4: testRegIPv4, IPv6: testRegIPv6},
			wantHost: testRegIPv6,
			wantAddr: addrV6,
			wantURI:  uriV6,
		},
		{
			name:         "POD_IP not set: the default",
			cfg:          registerConfig(Sbi{RegisterIPv4: envPodIP}),
			want:         sbiaddr.RegisterAddrs{IPv4: NRF_DEFAULT_IPV4},
			wantHost:     NRF_DEFAULT_IPV4,
			wantAddr:     addrDefault,
			wantURI:      "http://127.0.0.1:29510",
			wantWarnings: true,
		},
		{
			name:         "invalid value: the default",
			cfg:          registerConfig(Sbi{RegisterIPv4: "10.0.0.256"}),
			want:         sbiaddr.RegisterAddrs{IPv4: NRF_DEFAULT_IPV4},
			wantHost:     NRF_DEFAULT_IPV4,
			wantAddr:     addrDefault,
			wantURI:      "http://127.0.0.1:29510",
			wantWarnings: true,
		},
		{
			name:         "invalid preferredFamily: IPv4",
			cfg:          registerConfig(Sbi{RegisterIPv4: testRegIPv4, RegisterIPv6: testRegIPv6, PreferredFamily: "IPv5"}),
			want:         sbiaddr.RegisterAddrs{IPv4: testRegIPv4, IPv6: testRegIPv6},
			wantHost:     testRegIPv4,
			wantAddr:     addrV4,
			wantURI:      uriV4,
			wantWarnings: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envPodIP, "")
			t.Setenv(envPodIPs, "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := tc.cfg.GetSbiRegisterAddrs(); got != tc.want {
				t.Errorf("GetSbiRegisterAddrs() = %+v, want %+v", got, tc.want)
			}
			if got := tc.cfg.GetSbiRegisterIP(); got != tc.wantHost {
				t.Errorf("GetSbiRegisterIP() = %q, want %q", got, tc.wantHost)
			}
			if got := tc.cfg.GetSbiRegisterAddr(); got != tc.wantAddr {
				t.Errorf("GetSbiRegisterAddr() = %q, want %q", got, tc.wantAddr)
			}
			got := tc.cfg.GetSbiUri()
			if got != tc.wantURI {
				t.Errorf("GetSbiUri() = %q, want %q", got, tc.wantURI)
			}
			if u, err := url.Parse(got + "/nnrf-nfm/v1/nf-instances/x"); err != nil || u.Port() == "" {
				t.Errorf("url.Parse(%q) = %v, %v; want a URI with a port", got, u, err)
			}
			_, warnings := tc.cfg.resolveSbiRegisterAddrs()
			_, prefErr := tc.cfg.parseSbiPreferredFamily()
			if gotWarnings := len(warnings) > 0 || prefErr != nil; gotWarnings != tc.wantWarnings {
				t.Errorf("warnings = %v, %v; want any: %v", warnings, prefErr, tc.wantWarnings)
			}
		})
	}
}

// TestGetSbiUriKeepsLegacyFormat checks that, for every registerIPv4 that is
// an IPv4 address, a host name or unset, the NRF's own URI is exactly what the
// previous concatenation, scheme + "://" + registerIPv4 + ":" + port, gave.
func TestGetSbiUriKeepsLegacyFormat(t *testing.T) {
	t.Setenv(envPodIP, "")
	for _, reg := range []string{"", "127.0.0.10", testRegIPv4, testRegFQDN, "nrf.sdcore.svc.cluster.local", "localhost"} {
		for _, port := range []int{0, 8000, NRF_DEFAULT_PORT} {
			for _, scheme := range []string{"", "http", "https"} {
				cfg := Config{Configuration: &Configuration{Sbi: &Sbi{Scheme: scheme, RegisterIPv4: reg, Port: port}}}
				legacyHost, legacyPort, legacyScheme := reg, port, scheme
				if legacyHost == "" {
					legacyHost = NRF_DEFAULT_IPV4
				}
				if legacyPort == 0 {
					legacyPort = NRF_DEFAULT_PORT
				}
				if legacyScheme == "" {
					legacyScheme = NRF_DEFAULT_SCHEME
				}
				legacyAddr := legacyHost + ":" + strconv.Itoa(legacyPort)
				if got := cfg.GetSbiUri(); got != legacyScheme+"://"+legacyAddr {
					t.Errorf("registerIPv4 %q, port %d, scheme %q: GetSbiUri() = %q, want legacy %q",
						reg, port, scheme, got, legacyScheme+"://"+legacyAddr)
				}
				if got := cfg.GetSbiRegisterAddr(); got != legacyAddr {
					t.Errorf("registerIPv4 %q, port %d: GetSbiRegisterAddr() = %q, want legacy %q", reg, port, got, legacyAddr)
				}
				if got := cfg.GetSbiRegisterIP(); got != legacyHost {
					t.Errorf("registerIPv4 %q: GetSbiRegisterIP() = %q, want legacy %q", reg, got, legacyHost)
				}
			}
		}
	}
}

func TestGetSbiBindingAddrFamilyNeutral(t *testing.T) {
	tests := []struct {
		name string
		sbi  Sbi
		env  map[string]string
		want string
	}{
		{name: "bindingAddr IPv6 wildcard", sbi: Sbi{BindingAddr: "::"}, want: "[::]:29510"},
		{name: "bindingAddr wins over bindingIPv4", sbi: Sbi{BindingAddr: testRegIPv6, BindingIPv4: "0.0.0.0", Port: 8000}, want: "[2001:db8:600::20]:8000"},
		{name: "bindingIPv4 alone, as before", sbi: Sbi{BindingIPv4: testRegIPv4}, want: addrV4},
		{name: "bindingAddr bracketed", sbi: Sbi{BindingAddr: "[" + testRegIPv6 + "]"}, want: addrV6},
		{
			name: "bindingAddr POD_IP holding IPv6",
			sbi:  Sbi{BindingAddr: envPodIP},
			env:  map[string]string{envPodIP: testRegIPv6},
			want: addrV6,
		},
		{
			name: "bindingAddr POD_IPS is a list: the dual-stack default",
			sbi:  Sbi{BindingAddr: envPodIPs},
			env:  map[string]string{envPodIPs: testRegIPv4 + "," + testRegIPv6},
			want: "0.0.0.0:29510",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg := Config{Configuration: &Configuration{Sbi: &tc.sbi}}
			if got := cfg.GetSbiBindingAddr(); got != tc.want {
				t.Fatalf("GetSbiBindingAddr() = %q, want %q", got, tc.want)
			}
		})
	}
}
