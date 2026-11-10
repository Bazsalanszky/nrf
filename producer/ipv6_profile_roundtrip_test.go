// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	openapiutils "github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	roundTripInstanceID = "22222222-2222-4222-8222-222222222222"
	roundTripServiceID  = "0"
	contentTypeJSON     = "application/json"
	ipv6EndpointV1      = "2001:db8:600::10"
	ipv6EndpointV2      = "2001:db8:600::11"
	ipv4EndpointV1      = "10.42.0.10"

	uriV4Endpoint      = "http://10.42.0.10:8000"
	uriV6Endpoint      = "http://[2001:db8:600::10]:8000"
	uriV6NonCanonical  = "http://[2001:DB8:600:0:0:0:0:10]:8000"
	storedIPv4AddrsKey = "ipv4addresses"
	storedIPv6AddrsKey = "ipv6addresses"
)

// bsonStoreDB is an in-memory datastore that keeps every document as BSON
// bytes and decodes it the way util/mongoapi does (the driver's default
// lower-cased keys, nested documents as maps: DefaultDocumentMap). A value
// that survives it survives a MongoDB round trip. Documents are keyed by
// their collection and the printed filter, which is enough for the scalar
// filters the management procedures use. RestfulAPIGetMany ignores its filter
// for NF profiles: the tests below store one profile and are about what the
// NRF does with it, not about MongoDB's query semantics.
type bsonStoreDB struct {
	dbadapter.DBInterface

	mu   sync.Mutex
	docs map[string]map[string][]byte
}

func newBSONStoreDB() *bsonStoreDB {
	return &bsonStoreDB{docs: map[string]map[string][]byte{}}
}

func (db *bsonStoreDB) put(t testing.TB, collName string, filter bson.M, doc map[string]any) {
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("bson.Marshal(%v): %v", doc, err)
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.docs[collName] == nil {
		db.docs[collName] = map[string][]byte{}
	}
	db.docs[collName][fmt.Sprint(filter)] = b
}

func decodeStored(b []byte) (map[string]any, error) {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(b)))
	dec.DefaultDocumentMap()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (db *bsonStoreDB) RestfulAPIPutOne(collName string, filter bson.M, putData map[string]any) (bool, error) {
	b, err := bson.Marshal(putData)
	if err != nil {
		return false, err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.docs[collName] == nil {
		db.docs[collName] = map[string][]byte{}
	}
	key := fmt.Sprint(filter)
	_, existed := db.docs[collName][key]
	db.docs[collName][key] = b
	return existed, nil
}

func (db *bsonStoreDB) RestfulAPIGetOne(collName string, filter bson.M) (map[string]any, error) {
	db.mu.Lock()
	b, ok := db.docs[collName][fmt.Sprint(filter)]
	db.mu.Unlock()
	if !ok {
		return nil, nil
	}
	return decodeStored(b)
}

func (db *bsonStoreDB) RestfulAPIGetMany(collName string, _ bson.M) ([]map[string]any, error) {
	if collName != collNfProfile {
		return nil, nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	var out []map[string]any
	for _, b := range db.docs[collName] {
		doc, err := decodeStored(b)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func (db *bsonStoreDB) RestfulAPIReplaceIfUnchanged(collName string, filter bson.M, _, putData map[string]any) (bool, error) {
	existed, err := db.RestfulAPIPutOne(collName, filter, putData)
	return existed, err
}

func (db *bsonStoreDB) RestfulAPIDeleteMany(collName string, filter bson.M) error {
	db.mu.Lock()
	delete(db.docs[collName], fmt.Sprint(filter))
	db.mu.Unlock()
	return nil
}

// addressFields projects an NF profile, in its JSON wire form, onto the
// fields that carry addresses: the profile's fqdn, ipv4Addresses and
// ipv6Addresses, and the fqdn, apiPrefix and ipEndPoints of each service in
// nfServices and nfServiceList.
func addressFields(t *testing.T, wire []byte) map[string]any {
	t.Helper()
	var profile map[string]any
	if err := json.Unmarshal(wire, &profile); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", wire, err)
	}
	service := func(s any) map[string]any {
		m, _ := s.(map[string]any)
		return map[string]any{"fqdn": m["fqdn"], "apiPrefix": m["apiPrefix"], "ipEndPoints": m["ipEndPoints"]}
	}
	out := map[string]any{
		"fqdn":          profile["fqdn"],
		"ipv4Addresses": profile["ipv4Addresses"],
		"ipv6Addresses": profile["ipv6Addresses"],
	}
	if list, ok := profile["nfServices"].([]any); ok {
		services := make([]any, len(list))
		for i, s := range list {
			services[i] = service(s)
		}
		out["nfServices"] = services
	}
	if list, ok := profile["nfServiceList"].(map[string]any); ok {
		services := map[string]any{}
		for k, s := range list {
			services[k] = service(s)
		}
		out["nfServiceList"] = services
	}
	return out
}

func wireJSON(t *testing.T, body any) []byte {
	t.Helper()
	buf, err := openapi.SetBody(body, contentTypeJSON)
	if err != nil {
		t.Fatalf("openapi.SetBody: %v", err)
	}
	return buf.Bytes()
}

func assertSameAddresses(t *testing.T, step string, want map[string]any, gotWire []byte) {
	t.Helper()
	if got := addressFields(t, gotWire); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: address fields changed\n got: %#v\nwant: %#v", step, got, want)
	}
}

// ausfProfileJSON is an AUSF registration as it arrives on the wire.
func ausfProfileJSON(ipv4Addresses, ipv6Addresses, endpoints string) string {
	service := fmt.Sprintf(`{"serviceInstanceId":%q,"serviceName":"nausf-auth","versions":[{"apiVersionInUri":"v1","apiFullVersion":"1.0.0"}],`+
		`"scheme":"http","nfServiceStatus":"REGISTERED","ipEndPoints":%s}`, roundTripServiceID, endpoints)
	addresses := ""
	if ipv4Addresses != "" {
		addresses += `"ipv4Addresses":` + ipv4Addresses + `,`
	}
	if ipv6Addresses != "" {
		addresses += `"ipv6Addresses":` + ipv6Addresses + `,`
	}
	return fmt.Sprintf(`{"nfInstanceId":%q,"nfType":"AUSF","nfStatus":"REGISTERED","plmnList":[{"mcc":"208","mnc":"93"}],%s`+
		`"nfServices":[%s],"nfServiceList":{%q:%s}}`, roundTripInstanceID, addresses, service, roundTripServiceID, service)
}

func endpointJSON(family, addr string, port int) string {
	return fmt.Sprintf(`{%q:%q,"transport":"TCP","port":%d}`, family, addr, port)
}

// TestIPv6AddressesSurviveRegisterUpdateAndDiscovery registers IPv4-only,
// IPv6-only and dual-stack profiles, reads them back with GET and discovery,
// patches IPv6 addresses into them, and reads them back again, all through a
// BSON datastore. The NRF must store and return ipv6Addresses and every
// IpEndPoint.ipv6Address exactly as the NF sent them, and a consumer must be
// able to build a bracketed URI from the discovered profile.
func TestIPv6AddressesSurviveRegisterUpdateAndDiscovery(t *testing.T) {
	v4 := endpointJSON("ipv4Address", ipv4EndpointV1, 8000)
	v6 := endpointJSON("ipv6Address", ipv6EndpointV1, 8000)
	tests := []struct {
		name       string
		profile    string
		wantStored map[string]any // lower-cased BSON keys, as MongoDB holds them
		wantURIv4  string         // SearchNFServiceURI preferring IPv4
		wantURIv6  string         // ... and preferring IPv6
		// wantPatchedURIv6 is the IPv6-preferring URI after the patch has
		// added an IPv6 endpoint; preferring IPv4 gives wantURIv4 or, with no
		// IPv4 endpoint, wantURIv6, as before the patch.
		wantPatchedURIv6 string
	}{
		{
			name:       "IPv4 only",
			profile:    ausfProfileJSON(`["`+ipv4EndpointV1+`"]`, "", "["+v4+"]"),
			wantStored: map[string]any{storedIPv4AddrsKey: bson.A{ipv4EndpointV1}, storedIPv6AddrsKey: nil},
			wantURIv4:  uriV4Endpoint,
			wantURIv6:  uriV4Endpoint,
			// The patch gave this profile its first IPv6 endpoint.
			wantPatchedURIv6: "http://[2001:db8:600::11]:8001",
		},
		{
			name:             "IPv6 only",
			profile:          ausfProfileJSON("", `["`+ipv6EndpointV1+`"]`, "["+v6+"]"),
			wantStored:       map[string]any{storedIPv4AddrsKey: nil, storedIPv6AddrsKey: bson.A{ipv6EndpointV1}},
			wantURIv4:        uriV6Endpoint,
			wantURIv6:        uriV6Endpoint,
			wantPatchedURIv6: uriV6Endpoint,
		},
		{
			name:             "dual stack",
			profile:          ausfProfileJSON(`["`+ipv4EndpointV1+`"]`, `["`+ipv6EndpointV1+`"]`, "["+v4+","+v6+"]"),
			wantStored:       map[string]any{storedIPv4AddrsKey: bson.A{ipv4EndpointV1}, storedIPv6AddrsKey: bson.A{ipv6EndpointV1}},
			wantURIv4:        uriV4Endpoint,
			wantURIv6:        uriV6Endpoint,
			wantPatchedURIv6: uriV6Endpoint,
		},
		{
			name: "IPv6 address in non-canonical form is kept verbatim",
			profile: ausfProfileJSON("", `["2001:DB8:600:0:0:0:0:10"]`,
				"["+endpointJSON("ipv6Address", "2001:DB8:600:0:0:0:0:10", 8000)+"]"),
			wantStored: map[string]any{storedIPv4AddrsKey: nil, storedIPv6AddrsKey: bson.A{"2001:DB8:600:0:0:0:0:10"}},
			// The NRF does not canonicalise, and neither does the consumer.
			wantURIv4:        uriV6NonCanonical,
			wantURIv6:        uriV6NonCanonical,
			wantPatchedURIv6: uriV6NonCanonical,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setRoundTripProfileExpiry(t)
			db := newBSONStoreDB()
			originalDBClient := dbadapter.DBClient
			dbadapter.DBClient = db
			t.Cleanup(func() {
				dbadapter.DBClient = originalDBClient
				profileCache.evict(roundTripInstanceID)
			})
			profileCache.evict(roundTripInstanceID)

			// Register (PUT), decoding the body the way the router does.
			var profile models.NFProfile
			if err := openapi.Decode(&profile, []byte(tc.profile), contentTypeJSON); err != nil {
				t.Fatalf("openapi.Decode: %v", err)
			}
			want := addressFields(t, []byte(tc.profile))
			rsp := HandleNFRegisterRequest(&httpwrapper.Request{Body: profile, Params: map[string]string{}, Query: url.Values{}, Header: http.Header{}})
			if rsp.Status != http.StatusCreated {
				t.Fatalf("register: status %d, body %s", rsp.Status, wireJSON(t, rsp.Body))
			}
			assertSameAddresses(t, "register response", want, wireJSON(t, rsp.Body))

			// What MongoDB holds.
			stored, err := db.RestfulAPIGetOne(collNfProfile, bson.M{fieldNfInstanceId: roundTripInstanceID})
			if err != nil || stored == nil {
				t.Fatalf("stored profile: %v, %v", stored, err)
			}
			for key, value := range tc.wantStored {
				if got := stored[key]; !reflect.DeepEqual(got, value) {
					t.Fatalf("stored %s = %#v, want %#v", key, got, value)
				}
			}

			assertGetAndDiscovery(t, "after register", want, tc.wantURIv4, tc.wantURIv6)

			// Update (PATCH): add an IPv6 profile address and an IPv6 endpoint.
			patch := fmt.Sprintf(`[{"op":"add","path":"/ipv6Addresses","value":[%q,%q]},`+
				`{"op":"add","path":"/nfServices/0/ipEndPoints/-","value":%s},`+
				`{"op":"add","path":"/nfServiceList/%s/ipEndPoints/-","value":%s}]`,
				ipv6EndpointV1, ipv6EndpointV2,
				endpointJSON("ipv6Address", ipv6EndpointV2, 8001),
				roundTripServiceID, endpointJSON("ipv6Address", ipv6EndpointV2, 8001))
			rsp = HandleUpdateNFInstanceRequest(&httpwrapper.Request{
				Params: map[string]string{"nfInstanceID": roundTripInstanceID},
				Body:   []byte(patch),
			})
			if rsp.Status != http.StatusOK {
				t.Fatalf("patch: status %d, body %s", rsp.Status, wireJSON(t, rsp.Body))
			}
			wantPatched := addressFields(t, []byte(tc.profile))
			wantPatched["ipv6Addresses"] = []any{ipv6EndpointV1, ipv6EndpointV2}
			newEndpoint := map[string]any{"ipv6Address": ipv6EndpointV2, "transport": "TCP", "port": float64(8001)}
			services := wantPatched["nfServices"].([]any)
			services[0].(map[string]any)["ipEndPoints"] = append(services[0].(map[string]any)["ipEndPoints"].([]any), newEndpoint)
			serviceList := wantPatched["nfServiceList"].(map[string]any)
			svc := serviceList[roundTripServiceID].(map[string]any)
			svc["ipEndPoints"] = append(svc["ipEndPoints"].([]any), newEndpoint)
			assertSameAddresses(t, "patch response", wantPatched, wireJSON(t, rsp.Body))

			assertGetAndDiscovery(t, "after patch", wantPatched, tc.wantURIv4, tc.wantPatchedURIv6)
		})
	}
}

// setRoundTripProfileExpiry enables profile expiry for one test, so that a
// registration does not first delete every profile of its NF type, and
// restores the configuration afterwards.
func setRoundTripProfileExpiry(t *testing.T) {
	t.Helper()
	previousExpiry := factory.NrfConfig.Configuration.NfProfileExpiryEnable
	previousKeepAlive := factory.NrfConfig.Configuration.NfKeepAliveTime
	factory.NrfConfig.Configuration.NfProfileExpiryEnable = true
	t.Cleanup(func() {
		factory.NrfConfig.Configuration.NfProfileExpiryEnable = previousExpiry
		factory.NrfConfig.Configuration.NfKeepAliveTime = previousKeepAlive
	})
}

func assertGetAndDiscovery(t *testing.T, step string, want map[string]any, wantURIv4, wantURIv6 string) {
	t.Helper()
	rsp := HandleGetNFInstanceRequest(&httpwrapper.Request{Params: map[string]string{"nfInstanceID": roundTripInstanceID}})
	if rsp.Status != http.StatusOK {
		t.Fatalf("%s: GET status %d", step, rsp.Status)
	}
	assertSameAddresses(t, step+": GET", want, wireJSON(t, rsp.Body))

	query := url.Values{}
	query.Set(queryParamTargetNFType, nfTypeAUSF)
	query.Set(queryParamRequesterNFType, "AMF")
	query.Set(queryParamServiceNames, "nausf-auth")
	rsp = HandleNFDiscoveryRequest(&httpwrapper.Request{Query: query})
	if rsp.Status != http.StatusOK {
		t.Fatalf("%s: discovery status %d", step, rsp.Status)
	}
	var result models.SearchResult
	if err := json.Unmarshal(wireJSON(t, rsp.Body), &result); err != nil {
		t.Fatalf("%s: decoding the search result: %v", step, err)
	}
	if len(result.NfInstances) != 1 {
		t.Fatalf("%s: discovery returned %d instances, want 1", step, len(result.NfInstances))
	}
	discovered := result.NfInstances[0]
	assertSameAddresses(t, step+": discovery", want, wireJSON(t, discovered))

	for preferIPv6, wantURI := range map[bool]string{false: wantURIv4, true: wantURIv6} {
		got := openapiutils.SearchNFServiceURI(&discovered, models.SERVICENAME_NAUSF_AUTH, models.NFSERVICESTATUS_REGISTERED, preferIPv6)
		if got != wantURI {
			t.Fatalf("%s: SearchNFServiceURI(preferIPv6=%v) = %q, want %q", step, preferIPv6, got, wantURI)
		}
	}
}

// TestURIListFallbackFindsProfilesBehindAnIPv6Nrf checks the discovery
// fallback, which reads NF instance IDs back out of the NRF's own Location
// URIs, when those URIs carry a bracketed IPv6 host.
func TestURIListFallbackFindsProfilesBehindAnIPv6Nrf(t *testing.T) {
	previousSbi := factory.NrfConfig.Configuration.Sbi
	sbi := *previousSbi
	sbi.RegisterIPv4 = "2001:db8:600::1"
	factory.NrfConfig.Configuration.Sbi = &sbi
	t.Cleanup(func() { factory.NrfConfig.Configuration.Sbi = previousSbi })

	href := factory.NrfConfig.GetSbiUri() + "/nnrf-nfm/v1/nf-instances/" + roundTripInstanceID
	if want := "http://[2001:db8:600::1]:8000/nnrf-nfm/v1/nf-instances/" + roundTripInstanceID; href != want {
		t.Fatalf("NF instance URI = %q, want %q", href, want)
	}
	if got := getNFInstanceIDFromURI(href); got != roundTripInstanceID {
		t.Fatalf("getNFInstanceIDFromURI(%q) = %q, want %q", href, got, roundTripInstanceID)
	}

	db := newBSONStoreDB()
	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = db
	t.Cleanup(func() {
		dbadapter.DBClient = originalDBClient
		profileCache.evict(roundTripInstanceID)
	})
	profileCache.evict(roundTripInstanceID)
	db.put(t, collUriList, bson.M{fieldNfType: nfTypeAUSF}, map[string]any{
		"nfType": nfTypeAUSF,
		"_link":  map[string]any{"item": []any{map[string]any{"href": href}}},
	})
	db.put(t, collNfProfile, bson.M{fieldNfInstanceId: roundTripInstanceID}, map[string]any{
		fieldNfInstanceId:  roundTripInstanceID,
		"nftype":           nfTypeAUSF,
		"nfstatus":         "REGISTERED",
		storedIPv6AddrsKey: bson.A{ipv6EndpointV1},
	})

	query := url.Values{}
	query.Set(queryParamTargetNFType, nfTypeAUSF)
	query.Set(queryParamRequesterNFType, "AMF")
	profiles, err := loadDiscoveryProfilesFromURIList(query)
	if err != nil {
		t.Fatalf("loadDiscoveryProfilesFromURIList: %v", err)
	}
	if len(profiles) != 1 || !reflect.DeepEqual(profiles[0].GetIpv6Addresses(), []string{ipv6EndpointV1}) {
		t.Fatalf("fallback profiles = %+v, want the one with ipv6Addresses [%s]", profiles, ipv6EndpointV1)
	}
}
