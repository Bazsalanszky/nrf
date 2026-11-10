// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/omec-project/nrf/logger"
)

// Ipv6ToInt - Convert an IPv6 prefix or address to *big.Int. ipv6 is either
// an Ipv6Prefix (TS 29.571: "2001:db8:abcd::/48"), which is what the
// ue-ipv6-prefix discovery parameter and the start and end of an
// Ipv6PrefixRange carry, or a bare IPv6 address. A prefix converts to its
// network address, a bare address to itself. A value that is neither is
// logged and converts to 0.
func Ipv6ToInt(ipv6 string) *big.Int {
	addr, err := parseIpv6PrefixOrAddr(ipv6)
	if err != nil {
		logger.UtilLog.Errorln("error", ipv6, err)
		return big.NewInt(0)
	}
	b := addr.As16()
	return new(big.Int).SetBytes(b[:])
}

// ipv6RangeKeyDigits is the number of decimal digits of the largest IPv6
// value, 2^128-1 (340282366920938463463374607431768211455).
const ipv6RangeKeyDigits = 39

// Ipv6ToRangeKey - Convert an IPv6 prefix or address (see Ipv6ToInt) to the
// decimal string the NRF stores for the bounds of an Ipv6PrefixRange and
// compares them with. The string is zero-padded to a fixed width, so that
// MongoDB's string comparison ($lte / $gte) orders keys like the addresses.
// Unpadded, 2001:db8:: ("42540766411282592856903984951653826560", 38
// digits) sorts after fc00:: ("334965454937798799971759379190646833152",
// 39 digits). EncodeGroupId uses the same technique. The padded string still
// parses with big.Int.SetString, so the discovery response converts it back
// unchanged.
func Ipv6ToRangeKey(ipv6 string) string {
	return fmt.Sprintf("%0*d", ipv6RangeKeyDigits, Ipv6ToInt(ipv6))
}

// parseIpv6PrefixOrAddr parses an Ipv6Prefix into its network address, or a
// bare address into itself. The previous implementation appended "/32" to the
// value before net.ParseCIDR, so an Ipv6Prefix, which already has a length,
// failed to parse and every prefix converted to 0.
func parseIpv6PrefixOrAddr(s string) (netip.Addr, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Addr{}, err
		}
		return prefix.Masked().Addr(), nil
	}
	return netip.ParseAddr(s)
}

// Ipv4ToInt - Convert Ipv4 string to int64
func Ipv4ToInt(ipv4 string) int64 {
	ipv4 = ipv4 + "/24"
	ip, _, err := net.ParseCIDR(ipv4)
	if err != nil {
		logger.UtilLog.Errorln("error", ip, err)
	}
	IPv4Int := big.NewInt(0)
	IPv4Int.SetBytes(ip)
	return IPv4Int.Int64()
}

// Ipv4IntToIpv4String - Convert Ipv4 int64 to string
func Ipv4IntToIpv4String(ip int64) string {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)).String()
}

// Ipv6IntToIpv6String - Convert Ipv6 *big.Int to string
func Ipv6IntToIpv6String(ip *big.Int) string {
	// FillBytes (not ip.Bytes, which strips leading zero bytes) guarantees a
	// full 16-byte address, since a leading zero byte is a normal IPv6 value.
	var buf [net.IPv6len]byte
	ip.FillBytes(buf[:])
	// netip.Addr.String (unlike net.IP.String) keeps IPv4-mapped values in
	// IPv6 notation instead of collapsing them to dotted-decimal IPv4.
	return netip.AddrFrom16(buf).String()
}

// EncodeGroupId - Encode GroupId (hex segments [10]-[3]-[3]-[25]) to a
// fixed-width decimal string ([13][3][3][31]) so lexicographic ordering of
// the result matches numeric ordering of the two converted hex components:
// 16^10-1 needs 13 decimal digits and 16^25-1 needs 31.
func EncodeGroupId(groupId string) string {
	externalGroupIdentitySplit := strings.Split(groupId, "-")

	groupServiceIdentifier, err := strconv.ParseInt(externalGroupIdentitySplit[0], 16, 64)
	if err != nil {
		logger.UtilLog.Errorln("invalid group service identifier", externalGroupIdentitySplit[0], err)
	}
	// 25-digit local-group component can exceed int64, so parse with big.Int.
	localGroupId, ok := new(big.Int).SetString(externalGroupIdentitySplit[3], 16)
	if !ok {
		logger.UtilLog.Errorln("invalid local group id", externalGroupIdentitySplit[3])
		localGroupId = new(big.Int)
	}

	networkComponent := externalGroupIdentitySplit[2]
	if len(networkComponent) == 2 {
		networkComponent = "0" + networkComponent
	}
	return fmt.Sprintf("%013d%s%s%031d",
		groupServiceIdentifier, externalGroupIdentitySplit[1], networkComponent, localGroupId)
}
