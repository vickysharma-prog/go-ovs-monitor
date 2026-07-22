// Package offload reports whether OVS flows are actually offloaded to hardware.
// Offload state lives in the datapath, not in OVSDB, so this reads it via
// "ovs-appctl dpctl/dump-flows", the same command used to confirm a BlueField /
// switchdev offload is real rather than a silent software fallback.
package offload

import (
	"fmt"
	"os/exec"
	"strings"
)

// Status summarises datapath offload for a host.
type Status struct {
	TotalFlows     int
	OffloadedFlows int
}

// Percent is the share of datapath flows that are offloaded to hardware.
func (s Status) Percent() float64 {
	if s.TotalFlows == 0 {
		return 0
	}
	return 100 * float64(s.OffloadedFlows) / float64(s.TotalFlows)
}

// Read collects datapath flow counts. It asks the datapath twice: once for all
// flows and once for only the offloaded ones (type=offloaded).
func Read() (Status, error) {
	total, err := dpctlCount()
	if err != nil {
		return Status{}, err
	}
	offloaded, err := dpctlCount("type=offloaded")
	if err != nil {
		return Status{}, err
	}
	return Status{TotalFlows: total, OffloadedFlows: offloaded}, nil
}

// dpctlCount runs "ovs-appctl dpctl/dump-flows [args...]" and counts the flow
// lines it prints.
func dpctlCount(args ...string) (int, error) {
	cmdArgs := append([]string{"dpctl/dump-flows"}, args...)
	out, err := exec.Command("ovs-appctl", cmdArgs...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ovs-appctl %s: %w: %s", strings.Join(cmdArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return countFlowLines(string(out)), nil
}

// countFlowLines counts datapath flow lines (those carrying per-flow counters),
// ignoring blank lines and any header text.
func countFlowLines(dump string) int {
	n := 0
	for _, line := range strings.Split(dump, "\n") {
		if strings.Contains(line, "packets:") && strings.Contains(line, "actions:") {
			n++
		}
	}
	return n
}
