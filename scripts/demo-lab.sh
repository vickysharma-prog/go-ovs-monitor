#!/usr/bin/env bash
#
# demo-lab.sh - stand up a small Open vSwitch bridge so go-ovs-monitor has
# something real to inspect: a userspace (netdev) bridge with two internal
# ports in separate network namespaces, plus a little ARP/ICMP traffic so the
# flow table and interface counters are non-zero.
#
# This runs OVS in userspace datapath mode, so it needs no kernel module and
# works in a container or Codespace.
#
#   sudo ./scripts/demo-lab.sh up      # create bridge br-demo + generate traffic
#   sudo ./scripts/demo-lab.sh down    # tear it all down
#
set -euo pipefail

BR="${BR:-br-demo}"
NS_A="ovsdemo-a"
NS_B="ovsdemo-b"
IP_A="10.20.0.1"
IP_B="10.20.0.2"

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
  pgrep -x ovs-vswitchd >/dev/null || \
    ovs-vswitchd --pidfile --detach --log-file >/dev/null 2>&1
}

up() {
  need_root
  start_ovs
  ovs-vsctl --may-exist add-br "$BR" -- set bridge "$BR" datapath_type=netdev

  # Two internal ports, each moved into its own netns with an IP on 10.20.0.0/24.
  for pair in "$NS_A:p-a:$IP_A" "$NS_B:p-b:$IP_B"; do
    ns="${pair%%:*}"; rest="${pair#*:}"; port="${rest%%:*}"; ip="${rest##*:}"
    ip netns add "$ns" 2>/dev/null || true
    ovs-vsctl --may-exist add-port "$BR" "$port" -- set interface "$port" type=internal
    ip link set "$port" netns "$ns"
    ip netns exec "$ns" ip addr add "$ip/24" dev "$port" 2>/dev/null || true
    ip netns exec "$ns" ip link set "$port" up
    ip netns exec "$ns" ip link set lo up
  done
  ip link set "$BR" up

  echo "generating ARP + ICMP across $BR ..."
  ip netns exec "$NS_A" ping -c 9 -i 0.2 "$IP_B" >/dev/null 2>&1 || true

  echo "demo lab ready: bridge $BR with ports p-a ($IP_A) and p-b ($IP_B)"
}

down() {
  need_root
  ovs-vsctl --if-exists del-br "$BR"
  ip netns del "$NS_A" 2>/dev/null || true
  ip netns del "$NS_B" 2>/dev/null || true
  echo "demo lab removed"
}

case "${1:-up}" in
  up)   up ;;
  down) down ;;
  *)    echo "usage: $0 [up|down]" >&2; exit 1 ;;
esac
