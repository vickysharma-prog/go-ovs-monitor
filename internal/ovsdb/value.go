package ovsdb

// OVSDB (RFC 7047) encodes column values in a compact JSON form that is not a
// plain scalar. This file decodes the three shapes we need:
//
//   - a UUID atom:   ["uuid", "36f1..."]
//   - a set:         ["set", [atom, atom, ...]]   (a 1-element set is just the atom)
//   - a map:         ["map", [[key, val], [key, val], ...]]
//
// Keeping this decoding in one place is what lets the rest of the code treat an
// Interface's "statistics" or a Bridge's "ports" column as ordinary Go values.

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// decodeUUID extracts the UUID string from an OVSDB ["uuid", "..."] atom.
func decodeUUID(v json.RawMessage) (string, error) {
	var pair []json.RawMessage
	if err := json.Unmarshal(v, &pair); err != nil || len(pair) != 2 {
		return "", fmt.Errorf("not a uuid atom: %s", v)
	}
	var tag, id string
	_ = json.Unmarshal(pair[0], &tag)
	_ = json.Unmarshal(pair[1], &id)
	if tag != "uuid" && tag != "named-uuid" {
		return "", fmt.Errorf("unexpected uuid tag %q", tag)
	}
	return id, nil
}

// decodeSet returns the members of an OVSDB set. OVSDB encodes a set with
// exactly one member as the bare atom (no ["set", ...] wrapper), so we handle
// both forms and always return a slice.
func decodeSet(v json.RawMessage) []json.RawMessage {
	var arr []json.RawMessage
	if err := json.Unmarshal(v, &arr); err == nil && len(arr) == 2 {
		var tag string
		if json.Unmarshal(arr[0], &tag) == nil && tag == "set" {
			var items []json.RawMessage
			if json.Unmarshal(arr[1], &items) == nil {
				return items
			}
		}
	}
	// Not a ["set", ...] wrapper => it is a single atom.
	return []json.RawMessage{v}
}

// decodeUUIDSet returns the UUID strings referenced by a set-of-uuids column
// (for example Bridge.ports or Port.interfaces).
func decodeUUIDSet(v json.RawMessage) []string {
	var out []string
	for _, item := range decodeSet(v) {
		if id, err := decodeUUID(item); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// decodeStringMap decodes an OVSDB ["map", [[k,v],...]] column into a Go map.
// Values that are numbers are rendered as their decimal string so callers can
// treat, e.g., an interface's statistics uniformly.
func decodeStringMap(v json.RawMessage) map[string]string {
	out := map[string]string{}
	var arr []json.RawMessage
	if err := json.Unmarshal(v, &arr); err != nil || len(arr) != 2 {
		return out
	}
	var tag string
	if json.Unmarshal(arr[0], &tag) != nil || tag != "map" {
		return out
	}
	var pairs [][]json.RawMessage
	if json.Unmarshal(arr[1], &pairs) != nil {
		return out
	}
	for _, p := range pairs {
		if len(p) != 2 {
			continue
		}
		var k string
		if json.Unmarshal(p[0], &k) != nil {
			continue
		}
		out[k] = rawToString(p[1])
	}
	return out
}

// decodeInt64Map is decodeStringMap for columns whose values are integers, such
// as Interface.statistics (rx_packets, tx_bytes, ...).
func decodeInt64Map(v json.RawMessage) map[string]int64 {
	out := map[string]int64{}
	for k, s := range decodeStringMap(v) {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			out[k] = n
		}
	}
	return out
}

// rawToString renders a JSON atom as a plain string (a quoted string without its
// quotes, a number as-is).
func rawToString(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v)
}

// RawString and StatsMap are exported so the watch command can decode the rows
// carried in an OVSDB monitor update, which arrive as raw column values.
func RawString(v json.RawMessage) string          { return rawToString(v) }
func StatsMap(v json.RawMessage) map[string]int64 { return decodeInt64Map(v) }
