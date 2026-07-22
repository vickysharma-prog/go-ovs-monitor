#!/usr/bin/env bash
#
# demo-lab.sh - stand up a small Open vSwitch bridge so go-ovs-monitor has
# something real to inspect: a bridge with two ports, a handful of OpenFlow
# rules, and some ARP/ICMP traffic so the flow table and interface counters are
# non-zero.
#
# It uses OVS's built-in userspace "dummy" datapath, which needs no kernel
# module, no /dev/net/tun and no network namespaces - so it reproduces anywhere,
# including inside a container or a Codespace / CI runner.
#
#   sudo ./scripts/demo-lab.sh up      # create bridge br-demo + generate traffic
#   sudo ./scripts/demo-lab.sh down    # tear it all down
#
set -euo pipefail

BR="${BR:-br-demo}"
MAC_A="0a:00:00:00:00:01"
MAC_B="0a:00:00:00:00:02"
IP_A="10.20.0.1"
IP_B="10.20.0.2"
COUNT="${COUNT:-9}"

need_root() { [ "$(id -u)" -eq 0 ] || { echo "run as root (sudo)"; exit 1; }; }

start_ovs() {
  mkdir -p /var/run/openvswitch /etc/openvswitch
  if ! pgrep -x ovsdb-server >/dev/null; then
    [ -f /etc/openvswitch/conf.db ] || \
      ovsdb-tool create /etc/openvswitch/conf.db /usr/share/openvswitch/vswitch.ovsschema
    ovsdb-server /etc/openvswitch/conf.db \
      --remote=punix:/var/run/openvswitch/db.sock \
      --remote=db:Open_vSwitch,Open_vSwitch,manager_options \
      --pidfile --detach --log-file >/dev/null 2>&1
    ovs-vsctl --no-wait init
  fi
  # Restart vswitchd with the dummy datapath enabled so bridges of
  # datapath_type=dummy work without any kernel support.
  if ! pgrep -f 'ovs-vswitchd.*enable-dummy' >/dev/null; then
    pkill -x ovs-vswitchd 2>/dev/null || true
    sleep 1
    ovs-vswitchd --enable-dummy=override --disable-system \
      --pidfile --detach --log-file -vconsole:off >/dev/null 2>&1
    sleep 1
  fi
  # Refresh interface statistics once per second so counters show up promptly.
  ovs-vsctl --no-wait set Open_vSwitch . other_config:stats-update-interval=1000
}

# inject sends one packet "into" a dummy port as if it had arrived there.
inject() { ovs-appctl netdev-dummy/receive "$1" "$2" >/dev/null 2>&1 || true; }

up() {
  need_root
  start_ovs

  ovs-vsctl --may-exist add-br "$BR" -- set bridge "$BR" datapath_type=dummy
  ovs-vsctl --may-exist add-port "$BR" p-a -- set interface p-a type=dummy ofport_request=1
  ovs-vsctl --may-exist add-port "$BR" p-b -- set interface p-b type=dummy ofport_request=2

  # Forward ARP normally, steer ICMP between the two ports, flood the rest.
  ovs-ofctl del-flows "$BR"
  ovs-ofctl add-flow "$BR" "priority=100,arp actions=NORMAL"
  ovs-ofctl add-flow "$BR" "priority=100,icmp,nw_dst=${IP_B} actions=output:2"
  ovs-ofctl add-flow "$BR" "priority=100,icmp,nw_dst=${IP_A} actions=output:1"
  ovs-ofctl add-flow "$BR" "priority=0 actions=NORMAL"

  echo "generating ARP + ICMP across $BR ..."
  inject p-a "in_port(1),eth(src=${MAC_A},dst=ff:ff:ff:ff:ff:ff),eth_type(0x0806),arp(sip=${IP_A},tip=${IP_B},op=1,sha=${MAC_A},tha=00:00:00:00:00:00)"
  inject p-b "in_port(2),eth(src=${MAC_B},dst=${MAC_A}),eth_type(0x0806),arp(sip=${IP_B},tip=${IP_A},op=2,sha=${MAC_B},tha=${MAC_A})"
  for _ in $(seq 1 "$COUNT"); do
    # echo request p-a -> p-b, echo reply p-b -> p-a
    inject p-a "in_port(1),eth(src=${MAC_A},dst=${MAC_B}),eth_type(0x0800),ipv4(src=${IP_A},dst=${IP_B},proto=1,tos=0,ttl=64,frag=no),icmp(type=8,code=0)"
    inject p-b "in_port(2),eth(src=${MAC_B},dst=${MAC_A}),eth_type(0x0800),ipv4(src=${IP_B},dst=${IP_A},proto=1,tos=0,ttl=64,frag=no),icmp(type=0,code=0)"
  done

  sleep 2  # let the periodic stats refresh land in OVSDB
  echo "demo lab ready: bridge $BR with ports p-a ($IP_A) and p-b ($IP_B)"
}

down() {
  need_root
  ovs-vsctl --if-exists del-br "$BR"
  echo "demo lab removed"
}

case "${1:-up}" in
  up)   up ;;
  down) down ;;
  *)    echo "usage: $0 [up|down]" >&2; exit 1 ;;
esac
