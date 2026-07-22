// Package openflow reads OpenFlow rules from a bridge. OpenFlow flows live in
// ovs-vswitchd, not in OVSDB, so unlike the ovsdb package this one shells out to
// ovs-ofctl (the standard way to read a bridge's flow table) and parses the
// result into a filterable structure.
package openflow

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Flow is one parsed OpenFlow rule.
type Flow struct {
	Table    int
	Priority int
	NPackets int64
	NBytes   int64
	Match    string // the match part, e.g. "ip,nw_dst=10.10.0.1" or "" for a wildcard
	Actions  string
	Raw      string
}

// Filter narrows a flow dump. A zero value keeps everything.
type Filter struct {
	Table int    // -1 means any table
	Match string // case-insensitive substring on the match field; "" means any
}

var (
	reTable    = regexp.MustCompile(`table=(\d+)`)
	rePriority = regexp.MustCompile(`priority=(\d+)`)
	reNPackets = regexp.MustCompile(`n_packets=(\d+)`)
	reNBytes   = regexp.MustCompile(`n_bytes=(\d+)`)
	reActions  = regexp.MustCompile(`actions=(.*)$`)
)

// Dump returns the flows on bridge, filtered by f. It runs
// "ovs-ofctl dump-flows <bridge>".
func Dump(bridge string, f Filter) ([]Flow, error) {
	out, err := exec.Command("ovs-ofctl", "dump-flows", bridge).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ovs-ofctl dump-flows %s: %w: %s", bridge, err, strings.TrimSpace(string(out)))
	}
	return parseAndFilter(string(out), f), nil
}

// parseAndFilter is separated from Dump so it can be unit-tested without OVS.
func parseAndFilter(dump string, f Filter) []Flow {
	var flows []Flow
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "table=") {
			continue
		}
		flow := parseLine(line)
		if f.Table >= 0 && flow.Table != f.Table {
			continue
		}
		if f.Match != "" && !strings.Contains(strings.ToLower(flow.Match), strings.ToLower(f.Match)) {
			continue
		}
		flows = append(flows, flow)
	}
	return flows
}

func parseLine(line string) Flow {
	flow := Flow{Raw: line, Table: -1}
	if m := reTable.FindStringSubmatch(line); m != nil {
		flow.Table = atoi(m[1])
	}
	if m := rePriority.FindStringSubmatch(line); m != nil {
		flow.Priority = atoi(m[1])
	}
	if m := reNPackets.FindStringSubmatch(line); m != nil {
		flow.NPackets = atoi64(m[1])
	}
	if m := reNBytes.FindStringSubmatch(line); m != nil {
		flow.NBytes = atoi64(m[1])
	}
	if m := reActions.FindStringSubmatch(line); m != nil {
		flow.Actions = strings.TrimSpace(m[1])
	}
	flow.Match = extractMatch(line)
	return flow
}

// extractMatch pulls the match tokens out of a flow line: everything after the
// comma-separated metadata (cookie/duration/table/n_packets/... and priority)
// and before "actions=".
func extractMatch(line string) string {
	head := line
	if idx := strings.Index(line, "actions="); idx >= 0 {
		head = line[:idx]
	}
	var keep []string
	for _, tok := range strings.Split(head, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		switch {
		case strings.HasPrefix(tok, "cookie="),
			strings.HasPrefix(tok, "duration="),
			strings.HasPrefix(tok, "table="),
			strings.HasPrefix(tok, "n_packets="),
			strings.HasPrefix(tok, "n_bytes="),
			strings.HasPrefix(tok, "idle_age="),
			strings.HasPrefix(tok, "hard_age="),
			strings.HasPrefix(tok, "priority="):
			continue
		}
		keep = append(keep, tok)
	}
	return strings.Join(keep, ",")
}

func atoi(s string) int     { n, _ := strconv.Atoi(s); return n }
func atoi64(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
