# go-ovs-monitor

A small Go CLI for inspecting a running **Open vSwitch** instance. It connects to
`ovsdb-server` over the **OVSDB management protocol** (JSON-RPC, RFC 7047) to read
bridges, ports and live interface counters, reads **OpenFlow** rules through
`ovs-ofctl` with table/match filtering, and reports **datapath hardware-offload**
status the way you would verify a DPU / switchdev offload.

It talks to OVS the same way OVN and ovn-kubernetes do — a direct OVSDB client,
not a wrapper around `ovs-vsctl`.

## Why

When you are bringing up VM networking on an OVS bridge (KubeVirt, libvirt, or a
DPU), you constantly need to answer: *which ports are on this bridge, are the
counters moving, what flows are installed, and is any of it offloaded to
hardware?* This tool answers those from one place, in scriptable form.

## Features

- **OVSDB protocol client** — connects to the ovsdb-server socket and runs
  `transact`/`select` on the `Bridge`, `Port` and `Interface` tables (no shelling
  out to `ovs-vsctl`).
- **Live monitoring** — uses the OVSDB `monitor` request to stream interface
  counter updates in real time.
- **Flow inspection** — dumps OpenFlow rules and filters by `--table` and
  `--match` (e.g. `icmp`, `arp`, `nw_dst=...`).
- **Offload reporting** — reads `hw-offload` from the `Open_vSwitch` table and
  counts offloaded datapath flows via `ovs-appctl dpctl/dump-flows type=offloaded`.
- **JSON output** — every command supports `--json` for piping into other tools.
- Single static binary, standard library only (no third-party dependencies).

## Install

```bash
go install github.com/vickysharma-prog/go-ovs-monitor@latest
# or from a clone:
make build      # produces ./go-ovs-monitor
```

Requires Go 1.24+. The `flows` and `offload` commands call `ovs-ofctl` /
`ovs-appctl`, so run them on a host that has Open vSwitch installed.

## Usage

```
go-ovs-monitor <command> [flags]

Commands:
  bridges                 list bridges, ports and interface counters (OVSDB)
  ports    --bridge NAME  show ports + interface statistics for one bridge
  flows    --bridge NAME  dump OpenFlow rules, filter by --table / --match
  offload                 report datapath hardware-offload status
  watch    --bridge NAME  stream live interface counter updates (OVSDB monitor)

Common flags:
  --db TARGET   ovsdb-server endpoint (default unix:/var/run/openvswitch/db.sock,
                or tcp:host:port)
  --json        machine-readable JSON output
```

## Try it in 30 seconds

The repo ships a demo lab that stands up a userspace OVS bridge with two ports and
a little ARP/ICMP traffic (no kernel module needed, works in a container/Codespace):

```bash
sudo ./scripts/demo-lab.sh up      # creates br-demo with p-a (10.20.0.1) / p-b (10.20.0.2)
sudo ./go-ovs-monitor bridges
sudo ./go-ovs-monitor flows --bridge br-demo --match icmp
sudo ./go-ovs-monitor offload
sudo ./scripts/demo-lab.sh down
```

## Example session

<!-- CAPTURED_OUTPUT -->

## How it works

OVS exposes two control surfaces, and this tool uses the right one for each job:

| Data | Source | How |
|---|---|---|
| Bridges, ports, interfaces, counters | **OVSDB** (`ovsdb-server`) | JSON-RPC `transact`/`select`; `monitor` for live updates |
| OpenFlow rules | **ovs-vswitchd** | `ovs-ofctl dump-flows`, parsed and filtered |
| Datapath offload state | **datapath** | `ovs-appctl dpctl/dump-flows [type=offloaded]` |

The OVSDB values (sets of UUID references, maps of statistics) are decoded in
`internal/ovsdb/value.go`; the table join that turns three OVSDB tables into a
bridge → ports → interfaces tree is in `internal/ovsdb/model.go`.

## Project layout

```
main.go                     CLI dispatch and output formatting
internal/ovsdb/             OVSDB JSON-RPC client, value decoding, table model
internal/openflow/          ovs-ofctl dump-flows wrapper, parser, filtering
internal/offload/           datapath offload status via ovs-appctl
scripts/demo-lab.sh         reproducible OVS bridge + traffic for a quick demo
```

## Development

```bash
make lint    # gofmt + go vet + go test
make test    # unit tests (no OVS required)
```

The parsing and OVSDB value decoding are unit-tested without a running OVS.

## License

MIT — see [LICENSE](LICENSE).
