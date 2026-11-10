// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"net"
	"strconv"
	"testing"
)

func TestGetSbiBindingAddr(t *testing.T) {
	const (
		defaultAddr = "0.0.0.0:29510"
		podIPEnv    = "POD_IP"
	)
	tests := []struct {
		name string
		cfg  Config
		env  map[string]string
		want string
	}{
		{name: "no configuration", cfg: Config{}, want: defaultAddr},
		{name: "no sbi section", cfg: Config{Configuration: &Configuration{}}, want: defaultAddr},
		{name: "empty bindingIPv4", cfg: sbiConfig("", 0), want: defaultAddr},
		{name: "IPv4 address and port", cfg: sbiConfig("192.0.2.30", 29511), want: "192.0.2.30:29511"},
		{name: "hostname", cfg: sbiConfig("nrf.sdcore.svc.cluster.local", 0), want: "nrf.sdcore.svc.cluster.local:29510"},
		{name: "IPv6 wildcard", cfg: sbiConfig("::", 0), want: "[::]:29510"},
		{name: "IPv6 address", cfg: sbiConfig("2001:db8::30", 29511), want: "[2001:db8::30]:29511"},
		{name: "already bracketed IPv6", cfg: sbiConfig("[2001:db8::31]", 0), want: "[2001:db8::31]:29510"},
		{
			name: "environment variable holding an IPv4 address",
			cfg:  sbiConfig(podIPEnv, 0),
			env:  map[string]string{podIPEnv: "10.42.0.30"},
			want: "10.42.0.30:29510",
		},
		{
			name: "environment variable holding an IPv6 address",
			cfg:  sbiConfig(podIPEnv, 0),
			env:  map[string]string{podIPEnv: "2001:db8::32"},
			want: "[2001:db8::32]:29510",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got := tc.cfg.GetSbiBindingAddr()
			if got != tc.want {
				t.Fatalf("GetSbiBindingAddr() = %q, want %q", got, tc.want)
			}
			if _, _, err := net.SplitHostPort(got); err != nil {
				t.Fatalf("net.SplitHostPort(%q) failed: %v", got, err)
			}
		})
	}
}

// TestSbiHostPortKeepsLegacyFormat checks that every non-IPv6 host gives exactly
// the string the previous host + ":" + port concatenation produced.
func TestSbiHostPortKeepsLegacyFormat(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "127.0.0.1", "10.0.0.1", "", "localhost", "nrf"} {
		for _, port := range []int{NRF_DEFAULT_PORT, 80, 0} {
			if got, legacy := sbiHostPort(host, port), host+":"+strconv.Itoa(port); got != legacy {
				t.Errorf("sbiHostPort(%q, %d) = %q, want legacy %q", host, port, got, legacy)
			}
		}
	}
}

func sbiConfig(bindingIPv4 string, port int) Config {
	return Config{Configuration: &Configuration{Sbi: &Sbi{BindingIPv4: bindingIPv4, Port: port}}}
}
