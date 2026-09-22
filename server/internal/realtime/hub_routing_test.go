package realtime

import (
	"encoding/json"
	"testing"
)

func TestTargetMatchesOnlyExplicitDeviceOrUser(t *testing.T) {
	alicePhone := clientIdentity{
		deviceID: "dev_alice_phone",
		userID:   "usr_alice",
	}
	aliceTablet := clientIdentity{
		deviceID: "dev_alice_tablet",
		userID:   "usr_alice",
	}
	bobPhone := clientIdentity{
		deviceID: "dev_bob_phone",
		userID:   "usr_bob",
	}

	deviceTarget := routeTargets{
		DeviceIDs: []string{"dev_alice_phone"},
	}
	if !targetMatches(alicePhone, deviceTarget) {
		t.Fatal("explicit destination device should match")
	}
	if targetMatches(aliceTablet, deviceTarget) {
		t.Fatal("same-account sibling device must not receive device-targeted event")
	}
	if targetMatches(bobPhone, deviceTarget) {
		t.Fatal("different account must not receive device-targeted event")
	}

	userTarget := routeTargets{
		UserIDs: []string{"usr_alice"},
	}
	if !targetMatches(alicePhone, userTarget) ||
		!targetMatches(aliceTablet, userTarget) {
		t.Fatal("same-account devices should receive user-targeted presence event")
	}
	if targetMatches(bobPhone, userTarget) {
		t.Fatal("different account must not receive user-targeted event")
	}
}

func TestTargetMatchesNeverFallsBackToGlobalBroadcast(t *testing.T) {
	client := clientIdentity{
		deviceID: "dev_1",
		userID:   "usr_1",
	}

	if targetMatches(client, routeTargets{}) {
		t.Fatal("empty targets must not become a global broadcast")
	}
	if targetMatches(
		clientIdentity{deviceID: "dev_1"},
		routeTargets{UserIDs: []string{"usr_1"}},
	) {
		t.Fatal("anonymous/local client must not match a user target")
	}
}

func TestBrokerEnvelopeKeepsRoutingMetadataOutOfClientEvent(t *testing.T) {
	envelope := brokerEnvelope{
		Targets: routeTargets{
			DeviceIDs: []string{"dev_secret"},
			UserIDs:   []string{"usr_secret"},
		},
		Event: Event{
			Type: "transfer.ready",
			Payload: map[string]any{
				"transferId": "tr_1",
			},
		},
	}

	wire, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	var decoded brokerEnvelope
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}

	clientPayload, err := json.Marshal(decoded.Event)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]any
	if err := json.Unmarshal(clientPayload, &public); err != nil {
		t.Fatal(err)
	}

	if _, ok := public["targets"]; ok {
		t.Fatal("broker routing metadata must not be exposed to WebSocket clients")
	}
	if got, _ := public["type"].(string); got != "transfer.ready" {
		t.Fatalf("unexpected event type %q", got)
	}
}

func TestCompactTargetsRemovesEmptyAndDuplicateValues(t *testing.T) {
	got := compactTargets([]string{"dev_1", "", "dev_1", "dev_2"})
	if len(got) != 2 || got[0] != "dev_1" || got[1] != "dev_2" {
		t.Fatalf("unexpected compact targets %#v", got)
	}
}
