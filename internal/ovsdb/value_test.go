package ovsdb

import (
	"encoding/json"
	"testing"
)

func TestDecodeUUID(t *testing.T) {
	got, err := decodeUUID(json.RawMessage(`["uuid","36f1c7a5-95fa-0000-0000-000000000001"]`))
	if err != nil {
		t.Fatalf("decodeUUID: %v", err)
	}
	if want := "36f1c7a5-95fa-0000-0000-000000000001"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := decodeUUID(json.RawMessage(`"not-a-uuid"`)); err == nil {
		t.Fatal("expected error for non-uuid atom")
	}
}

func TestDecodeUUIDSet(t *testing.T) {
	// A multi-member set is wrapped in ["set", [...]].
	multi := json.RawMessage(`["set",[["uuid","aaaa"],["uuid","bbbb"]]]`)
	if got := decodeUUIDSet(multi); len(got) != 2 || got[0] != "aaaa" || got[1] != "bbbb" {
		t.Fatalf("multi set: got %v", got)
	}
	// A single-member set is encoded as the bare atom.
	single := json.RawMessage(`["uuid","cccc"]`)
	if got := decodeUUIDSet(single); len(got) != 1 || got[0] != "cccc" {
		t.Fatalf("single set: got %v", got)
	}
}

func TestDecodeInt64Map(t *testing.T) {
	stats := json.RawMessage(`["map",[["rx_packets",9],["tx_packets",9],["rx_bytes",882]]]`)
	got := decodeInt64Map(stats)
	if got["rx_packets"] != 9 || got["tx_packets"] != 9 || got["rx_bytes"] != 882 {
		t.Fatalf("stats: got %v", got)
	}
}

func TestDecodeOptScalar(t *testing.T) {
	// unset optional column -> empty set -> ""
	if got := decodeOptScalar(json.RawMessage(`["set",[]]`)); got != "" {
		t.Fatalf("empty optional: got %q, want \"\"", got)
	}
	// present as a bare atom
	if got := decodeOptScalar(json.RawMessage(`"up"`)); got != "up" {
		t.Fatalf("present optional: got %q, want \"up\"", got)
	}
}

func TestDecodeStringMap(t *testing.T) {
	oc := json.RawMessage(`["map",[["hw-offload","true"]]]`)
	if got := decodeStringMap(oc)["hw-offload"]; got != "true" {
		t.Fatalf("hw-offload: got %q", got)
	}
	// An empty map must not panic.
	if got := decodeStringMap(json.RawMessage(`["map",[]]`)); len(got) != 0 {
		t.Fatalf("empty map: got %v", got)
	}
}
