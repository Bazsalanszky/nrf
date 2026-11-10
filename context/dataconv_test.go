// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"math/big"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"testing"
)

const (
	tAddrDoc       = "2001:db8::"
	tAddrDocOne    = "2001:db8::1"
	tAddrLoopback  = "::1"
	tAddrMax       = "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"
	tPrefix48      = "2001:db8:abcd::/48"
	tPrefix48Addr  = "2001:db8:abcd::"
	tPrefix64Addr  = "2001:db8:abcd:12::"
	tPrefixULA     = "fd00:1::/64"
	tPrefixULAAddr = "fd00:1::"
)

func mustBig(t *testing.T, addr string) *big.Int {
	t.Helper()
	b := netip.MustParseAddr(addr).As16()
	return new(big.Int).SetBytes(b[:])
}

func TestIpv6ToInt(t *testing.T) {
	tests := []struct {
		in   string
		want string // the address the result must equal
	}{
		// Ipv6Prefix values, as ue-ipv6-prefix and Ipv6PrefixRange carry them.
		// They all converted to 0 before.
		{in: "2001:db8::/32", want: tAddrDoc},
		{in: tPrefix48, want: tPrefix48Addr},
		{in: "2001:db8:abcd:12::/64", want: tPrefix64Addr},
		{in: tPrefixULA, want: tPrefixULAAddr},
		{in: "2001:db8:abcd:12::1/64", want: tPrefix64Addr}, // host bits are masked
		{in: "2001:db8::5/128", want: "2001:db8::5"},
		{in: "::/0", want: "::"},
		{in: " 2001:db8::/32 ", want: tAddrDoc},
		// Bare addresses convert as before.
		{in: tAddrDocOne, want: tAddrDocOne},
		{in: tAddrLoopback, want: tAddrLoopback},
		{in: tAddrMax, want: tAddrMax},
		// Invalid values convert to 0, as before.
		{in: "", want: "::"},
		{in: "not-an-address", want: "::"},
		{in: "2001:db8::/129", want: "::"},
		{in: "2001:db8::/32/32", want: "::"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got, want := Ipv6ToInt(tc.in), mustBig(t, tc.want); got.Cmp(want) != 0 {
				t.Fatalf("Ipv6ToInt(%q) = %s (%s), want %s (%s)",
					tc.in, got, Ipv6IntToIpv6String(got), want, tc.want)
			}
		})
	}
}

// legacyIpv6ToInt is the previous implementation.
func legacyIpv6ToInt(ipv6 string) *big.Int {
	ip, _, err := net.ParseCIDR(ipv6 + "/32")
	if err != nil {
		return new(big.Int)
	}
	return new(big.Int).SetBytes(ip)
}

// TestIpv6ToIntKeepsLegacyBareAddresses checks that a bare address, the only
// input the previous implementation converted, gives the same value as before,
// so profiles stored with such values still compare the same way.
func TestIpv6ToIntKeepsLegacyBareAddresses(t *testing.T) {
	for _, addr := range []string{"::", tAddrLoopback, tAddrDoc, tAddrDocOne, tPrefix64Addr, "fd00::1", "fe80::1", "ffff::", "10.0.0.1", "::ffff:10.0.0.1"} {
		if got, legacy := Ipv6ToInt(addr), legacyIpv6ToInt(addr); got.Cmp(legacy) != 0 {
			t.Errorf("Ipv6ToInt(%q) = %s, legacy %s", addr, got, legacy)
		}
	}
}

func TestIpv6ToIntRoundTrip(t *testing.T) {
	for in, want := range map[string]string{
		tPrefix48:   tPrefix48Addr,
		tAddrDocOne: tAddrDocOne,
		tPrefixULA:  tPrefixULAAddr,
	} {
		if got := Ipv6IntToIpv6String(Ipv6ToInt(in)); got != want {
			t.Errorf("Ipv6IntToIpv6String(Ipv6ToInt(%q)) = %q, want %q", in, got, want)
		}
	}
}

// TestIpv4ToIntDecimalOrderIsNumeric pins a property the ue-ipv4-address
// discovery relies on: the BSF IPv4 range bounds and the queried address are
// stored and compared as decimal strings, with MongoDB's string ordering.
// That ordering is only numeric because Ipv4ToInt converts the 16-byte,
// IPv4-mapped form of the address, which gives every IPv4 address a decimal
// string of the same length (15 digits).
func TestIpv4ToIntDecimalOrderIsNumeric(t *testing.T) {
	addrs := []string{
		"0.0.0.0", "0.0.0.9", "0.0.0.10", "1.2.3.4", "9.255.255.255", "10.0.0.0", "10.0.0.1",
		"99.0.0.0", "100.0.0.0", "172.16.0.1", "192.168.1.1", "255.255.255.255",
	}
	keys := make([]string, len(addrs))
	for i, a := range addrs {
		keys[i] = strconv.FormatInt(Ipv4ToInt(a), 10)
		if len(keys[i]) != len(keys[0]) {
			t.Fatalf("Ipv4ToInt(%q) = %s has %d digits, %q has %d", a, keys[i], len(keys[i]), addrs[0], len(keys[0]))
		}
		if back := Ipv4IntToIpv4String(Ipv4ToInt(a)); back != a {
			t.Fatalf("Ipv4IntToIpv4String(Ipv4ToInt(%q)) = %q", a, back)
		}
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("decimal strings of numerically ordered addresses are not ordered as strings: %v", keys)
	}
}
