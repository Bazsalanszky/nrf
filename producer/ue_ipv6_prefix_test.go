// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"net/url"
	"testing"

	"github.com/omec-project/nrf/context"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	testUeIpv6Prefix    = "2001:db8:abcd:12::/64"
	testUeIpv6PrefixKey = "042540766464452359661416384011178082304" // 2001:db8:abcd:12::, 39 digits
)

// ipv6RangeBounds returns the $lte and $gte operands of an ue-ipv6-prefix
// range condition.
func ipv6RangeBounds(t *testing.T, cond bson.M) (lte, gte any) {
	t.Helper()
	ranges, ok := cond[fieldBsfInfoIpv6PrefixRanges].(bson.M)
	if !ok {
		t.Fatalf("no %s condition in %#v", fieldBsfInfoIpv6PrefixRanges, cond)
	}
	elem, ok := ranges[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("no %s in %#v", mongoOpElemMatch, ranges)
	}
	return elem[fieldStart].(bson.M)[mongoOpLte], elem[fieldEnd].(bson.M)[mongoOpGte]
}

// TestUeIpv6PrefixFilterComparesRangeKeys checks that ue-ipv6-prefix
// discovery, in the plain and the complexQuery form, compares the range
// bounds with the queried prefix's fixed-width key, the encoding
// copyBsfInfo stores them in. It used to compare them with "0".
func TestUeIpv6PrefixFilterComparesRangeKeys(t *testing.T) {
	if got := context.Ipv6ToRangeKey(testUeIpv6Prefix); got != testUeIpv6PrefixKey {
		t.Fatalf("Ipv6ToRangeKey(%q) = %q, want %q", testUeIpv6Prefix, got, testUeIpv6PrefixKey)
	}

	query := url.Values{}
	query.Set(queryParamUeIpv6Prefix, testUeIpv6Prefix)
	filter := bson.M{mongoOpAnd: []bson.M{}}
	handleUeIpv6Prefix(query, filter, nfTypeBSF)
	conds := filter[mongoOpAnd].([]bson.M)
	if len(conds) != 1 {
		t.Fatalf("plain query: got %d conditions, want 1", len(conds))
	}
	alternatives := conds[0][mongoOpOr].([]bson.M)
	if lte, gte := ipv6RangeBounds(t, alternatives[0]); lte != testUeIpv6PrefixKey || gte != testUeIpv6PrefixKey {
		t.Fatalf("plain query: bounds compared with %v / %v, want %s", lte, gte, testUeIpv6PrefixKey)
	}

	complexFilter := bson.M{mongoOpAnd: []bson.M{}}
	addIpv6PrefixFilter(map[string]*AtomElem{queryParamUeIpv6Prefix: {value: testUeIpv6Prefix}}, complexFilter, mongoOpAnd, nfTypeBSF)
	conds = complexFilter[mongoOpAnd].([]bson.M)
	if len(conds) != 1 {
		t.Fatalf("complexQuery: got %d conditions, want 1", len(conds))
	}
	if lte, gte := ipv6RangeBounds(t, conds[0]); lte != testUeIpv6PrefixKey || gte != testUeIpv6PrefixKey {
		t.Fatalf("complexQuery: bounds compared with %v / %v, want %s", lte, gte, testUeIpv6PrefixKey)
	}
}

// TestHandleBSFIpConversionDecodesRangeKeys checks that the discovery response
// turns stored, zero-padded range keys back into addresses.
func TestHandleBSFIpConversionDecodesRangeKeys(t *testing.T) {
	bsfInfo := models.NewBsfInfo()
	bsfInfo.SetIpv6PrefixRanges([]models.Ipv6PrefixRange{
		*models.NewIpv6PrefixRange(context.Ipv6ToRangeKey("2001:db8::/32"), context.Ipv6ToRangeKey("2001:db8:ffff::/48")),
	})
	profiles := []models.NFProfileDiscovery{{NfInstanceId: "bsf-1", NfType: models.NFTYPE_BSF, BsfInfo: bsfInfo}}
	query := url.Values{}
	query.Set(queryParamTargetNFType, nfTypeBSF)
	handleBSFIpConversion(query, profiles)

	got := profiles[0].BsfInfo.GetIpv6PrefixRanges()[0]
	if got.GetStart() != "2001:db8::" || got.GetEnd() != "2001:db8:ffff::" {
		t.Fatalf("decoded range = %s .. %s, want 2001:db8:: .. 2001:db8:ffff::", got.GetStart(), got.GetEnd())
	}
}
