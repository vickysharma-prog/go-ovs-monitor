// Command go-ovs-monitor inspects a running Open vSwitch instance: it lists
// bridges/ports and their interface statistics over the OVSDB protocol, dumps
// and filters OpenFlow rules, reports datapath hardware-offload status, and can
// watch interface counters live.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vickysharma-prog/go-ovs-monitor/internal/offload"
	"github.com/vickysharma-prog/go-ovs-monitor/internal/openflow"
	"github.com/vickysharma-prog/go-ovs-monitor/internal/ovsdb"
)

const defaultDB = "unix:/var/run/openvswitch/db.sock"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "bridges":
		err = cmdBridges(args)
	case "ports":
		err = cmdPorts(args)
	case "flows":
		err = cmdFlows(args)
	case "offload":
		err = cmdOffload(args)
	case "watch":
		err = cmdWatch(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `go-ovs-monitor - inspect Open vSwitch over OVSDB + OpenFlow

Usage:
  go-ovs-monitor <command> [flags]

Commands:
  bridges                 list bridges, ports and interface counters (OVSDB)
  ports    --bridge NAME  show ports + interface statistics for one bridge
  flows    --bridge NAME  dump OpenFlow rules, filter by --table / --match
  offload                 report datapath hardware-offload status
  watch    --bridge NAME  stream live interface counter updates (OVSDB monitor)

Common flags:
  --db TARGET    ovsdb-server endpoint (default unix:/var/run/openvswitch/db.sock,
                 or tcp:host:port)
  --json         machine-readable JSON output
`)
}

// dial opens the OVSDB connection shared by the OVSDB-backed commands.
func dial(db string) (*ovsdb.Client, error) {
	return ovsdb.Dial(db)
}

func cmdBridges(args []string) error {
	fs := flag.NewFlagSet("bridges", flag.ExitOnError)
	db := fs.String("db", defaultDB, "ovsdb-server endpoint")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)

	c, err := dial(*db)
	if err != nil {
		return err
	}
	defer c.Close()

	bridges, err := c.Bridges()
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(bridges)
	}
	for _, b := range bridges {
		dp := b.DatapathType
		if dp == "" {
			dp = "system"
		}
		fmt.Printf("bridge %s (datapath_type=%s, %d ports)\n", b.Name, dp, len(b.Ports))
		for _, p := range b.Ports {
			for _, i := range p.Interfaces {
				fmt.Printf("  %-16s type=%-8s rx=%d/%dB tx=%d/%dB link=%s\n",
					i.Name, orDash(i.Type),
					i.Statistics["rx_packets"], i.Statistics["rx_bytes"],
					i.Statistics["tx_packets"], i.Statistics["tx_bytes"],
					orDash(i.LinkState))
			}
		}
	}
	if len(bridges) == 0 {
		fmt.Println("(no bridges)")
	}
	return nil
}

func cmdPorts(args []string) error {
	fs := flag.NewFlagSet("ports", flag.ExitOnError)
	db := fs.String("db", defaultDB, "ovsdb-server endpoint")
	bridge := fs.String("bridge", "", "bridge name (required)")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)
	if *bridge == "" {
		return fmt.Errorf("--bridge is required")
	}

	c, err := dial(*db)
	if err != nil {
		return err
	}
	defer c.Close()

	bridges, err := c.Bridges()
	if err != nil {
		return err
	}
	for _, b := range bridges {
		if b.Name != *bridge {
			continue
		}
		if *asJSON {
			return printJSON(b)
		}
		fmt.Printf("ports on %s:\n", b.Name)
		for _, p := range b.Ports {
			fmt.Printf("  port %s\n", p.Name)
			for _, i := range p.Interfaces {
				fmt.Printf("    %-16s type=%-8s admin=%s link=%s rx_pkts=%d tx_pkts=%d\n",
					i.Name, orDash(i.Type), orDash(i.AdminState), orDash(i.LinkState),
					i.Statistics["rx_packets"], i.Statistics["tx_packets"])
			}
		}
		return nil
	}
	return fmt.Errorf("bridge %q not found", *bridge)
}

func cmdFlows(args []string) error {
	fs := flag.NewFlagSet("flows", flag.ExitOnError)
	bridge := fs.String("bridge", "", "bridge name (required)")
	table := fs.Int("table", -1, "only flows in this table (default: all)")
	match := fs.String("match", "", "only flows whose match contains this substring (e.g. icmp, arp)")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)
	if *bridge == "" {
		return fmt.Errorf("--bridge is required")
	}

	flows, err := openflow.Dump(*bridge, openflow.Filter{Table: *table, Match: *match})
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(flows)
	}
	fmt.Printf("%d flow(s) on %s:\n", len(flows), *bridge)
	for _, f := range flows {
		fmt.Printf("  table=%-2d prio=%-5d n_packets=%-8d match=%-28s actions=%s\n",
			f.Table, f.Priority, f.NPackets, orDash(f.Match), f.Actions)
	}
	return nil
}

func cmdOffload(args []string) error {
	fs := flag.NewFlagSet("offload", flag.ExitOnError)
	db := fs.String("db", defaultDB, "ovsdb-server endpoint")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)

	c, err := dial(*db)
	if err != nil {
		return err
	}
	defer c.Close()

	enabled, err := c.HWOffloadEnabled()
	if err != nil {
		return err
	}
	status, err := offload.Read()
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(struct {
			HWOffloadEnabled bool           `json:"hw_offload_enabled"`
			Datapath         offload.Status `json:"datapath"`
		}{enabled, status})
	}
	fmt.Printf("hw-offload config : %v\n", enabled)
	fmt.Printf("datapath flows    : %d total, %d offloaded (%.0f%%)\n",
		status.TotalFlows, status.OffloadedFlows, status.Percent())
	if !enabled && status.OffloadedFlows == 0 {
		fmt.Println("note: running a software datapath (no hardware offload) - expected without a DPU/switchdev NIC")
	}
	return nil
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	db := fs.String("db", defaultDB, "ovsdb-server endpoint")
	_ = fs.Parse(args)

	c, err := dial(*db)
	if err != nil {
		return err
	}
	defer c.Close()

	// Ask ovsdb-server to monitor the Interface table's name + statistics.
	req := map[string]interface{}{
		"Interface": map[string]interface{}{
			"columns": []string{"name", "statistics"},
		},
	}
	initial, err := c.Monitor("Open_vSwitch", req)
	if err != nil {
		return err
	}
	fmt.Println("watching interface counters (Ctrl-C to stop)...")
	printInterfaceUpdate("initial", initial)
	for {
		params, err := c.NextUpdate()
		if err != nil {
			return err
		}
		// params is [monitor-id, table-updates]; take the table-updates element.
		var arr []json.RawMessage
		if json.Unmarshal(params, &arr) == nil && len(arr) == 2 {
			printInterfaceUpdate("update", arr[1])
		}
	}
}

// printInterfaceUpdate renders the Interface rows in an OVSDB table-updates
// object, showing each interface's current rx/tx packet counters.
func printInterfaceUpdate(kind string, tableUpdates json.RawMessage) {
	var upd struct {
		Interface map[string]struct {
			New map[string]json.RawMessage `json:"new"`
		} `json:"Interface"`
	}
	if json.Unmarshal(tableUpdates, &upd) != nil {
		return
	}
	names := make([]string, 0, len(upd.Interface))
	rows := map[string]string{}
	for _, row := range upd.Interface {
		if row.New == nil {
			continue
		}
		name := ovsdb.RawString(row.New["name"])
		stats := ovsdb.StatsMap(row.New["statistics"])
		names = append(names, name)
		rows[name] = fmt.Sprintf("rx_pkts=%d tx_pkts=%d", stats["rx_packets"], stats["tx_packets"])
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("[%s] %-16s %s\n", kind, n, rows[n])
	}
}

func printJSON(v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
