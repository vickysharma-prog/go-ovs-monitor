package openflow

import "testing"

// A representative ovs-ofctl dump-flows output: a wildcard NORMAL rule plus two
// matched rules (ARP and ICMP), the same shape captured in real OVS labs.
const sampleDump = `
 cookie=0x0, duration=120.5s, table=0, n_packets=18, n_bytes=1764, priority=0 actions=NORMAL
 cookie=0x0, duration=110.2s, table=0, n_packets=2, n_bytes=84, priority=100,arp actions=NORMAL
 cookie=0x0, duration=110.2s, table=0, n_packets=9, n_bytes=882, priority=100,icmp,nw_dst=10.10.0.1 actions=output:3
`

func TestParseAndFilter_All(t *testing.T) {
	flows := parseAndFilter(sampleDump, Filter{Table: -1})
	if len(flows) != 3 {
		t.Fatalf("expected 3 flows, got %d", len(flows))
	}
	// The wildcard rule has an empty match and NORMAL action.
	if flows[0].Match != "" || flows[0].Actions != "NORMAL" || flows[0].NPackets != 18 {
		t.Fatalf("row 0 parsed wrong: %+v", flows[0])
	}
	// The ICMP rule keeps only real match tokens (not table/priority/counters).
	if flows[2].Match != "icmp,nw_dst=10.10.0.1" {
		t.Fatalf("icmp match parsed wrong: %q", flows[2].Match)
	}
	if flows[2].NPackets != 9 || flows[2].Actions != "output:3" {
		t.Fatalf("icmp row parsed wrong: %+v", flows[2])
	}
}

func TestParseAndFilter_MatchFilter(t *testing.T) {
	flows := parseAndFilter(sampleDump, Filter{Table: -1, Match: "icmp"})
	if len(flows) != 1 {
		t.Fatalf("expected 1 icmp flow, got %d", len(flows))
	}
	if flows[0].NPackets != 9 {
		t.Fatalf("wrong icmp flow: %+v", flows[0])
	}
}

func TestParseAndFilter_TableFilter(t *testing.T) {
	if got := parseAndFilter(sampleDump, Filter{Table: 5}); len(got) != 0 {
		t.Fatalf("expected no flows in table 5, got %d", len(got))
	}
	if got := parseAndFilter(sampleDump, Filter{Table: 0}); len(got) != 3 {
		t.Fatalf("expected 3 flows in table 0, got %d", len(got))
	}
}
