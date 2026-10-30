#!/bin/bash
# P0 network harness: a monitoring host behind a router, and "the
# internet" behind that, in three network namespaces. Needs root (sudo).
#
#   ukp0   the monitoring host        10.99.0.2/24, gateway 10.99.0.1
#   ukp0r  the router                 10.99.0.1 / 10.99.1.1
#   ukp0i  the internet               targets 198.51.100-108.10:8080, 203.0.113.10:8080,
#                                     webhook 192.0.2.10:19111, DNS 198.18.0.53
#
# harness.sh up | down | cut | restore | dns-cut | dns-restore |
#            gw-ping off|on | host-down IP | host-up IP | ns-exec CMD...
# The rig binary (scripts/p0/rig) must be built to $RIG.
set -eu
RIG=${RIG:-/tmp/p0rig}
LOGS=${LOGS:-/tmp/p0}
S="sudo -n"
NS=ukp0; R=ukp0r; I=ukp0i
NETS="198.51.0.0/16 203.0.113.0/24 192.0.2.0/24 198.18.0.0/24"
nsx() { $S ip netns exec "$@"; }
case "${1:-}" in
up)
  for n in $NS $R $I; do $S ip netns add $n; nsx $n ip link set lo up; done
  $S ip link add p0a netns $NS type veth peer name p0b netns $R
  $S ip link add p0c netns $R type veth peer name p0d netns $I
  nsx $NS ip addr add 10.99.0.2/24 dev p0a; nsx $NS ip link set p0a up
  nsx $NS ip route add default via 10.99.0.1
  nsx $R ip addr add 10.99.0.1/24 dev p0b; nsx $R ip link set p0b up
  nsx $R ip addr add 10.99.1.1/24 dev p0c; nsx $R ip link set p0c up
  nsx $R sysctl -qw net.ipv4.ip_forward=1
  nsx $I ip addr add 10.99.1.2/24 dev p0d; nsx $I ip link set p0d up
  nsx $I ip route add default via 10.99.1.1
  for net in $NETS; do nsx $R ip route add $net via 10.99.1.2; done
  for k in $(seq 100 108); do nsx $I ip addr add 198.51.$k.10/32 dev lo; done
  nsx $I ip addr add 203.0.113.10/32 dev lo
  nsx $I ip addr add 192.0.2.10/32 dev lo
  nsx $I ip addr add 198.18.0.53/32 dev lo
  $S mkdir -p /etc/netns/$NS
  echo "nameserver 198.18.0.53" | $S tee /etc/netns/$NS/resolv.conf >/dev/null
  mkdir -p $LOGS
  for k in $(seq 100 108); do
    nsx $I sudo -u "$(id -un)" $RIG target 198.51.$k.10:8080 $LOGS/target.log >/dev/null 2>&1 &
  done
  nsx $I sudo -u "$(id -un)" $RIG target 203.0.113.10:8080 $LOGS/target.log >/dev/null 2>&1 &
  nsx $I sudo -u "$(id -un)" $RIG hook 192.0.2.10:19111 $LOGS/hook.jsonl >/dev/null 2>&1 &
  nsx $I $RIG dns 198.18.0.53:53 203.0.113.10 >/dev/null 2>&1 &
  sleep 0.5
  ;;
down)
  for n in $I $R $NS; do
    for p in $($S ip netns pids $n 2>/dev/null); do $S kill -9 $p 2>/dev/null || true; done
  done
  for n in $NS $R $I; do $S ip netns del $n 2>/dev/null || true; done
  $S rm -rf /etc/netns/$NS
  ;;
# The uplink beyond the router dies: packets vanish (timeouts), the
# router itself still answers.
cut) for net in $NETS; do nsx $R ip route replace blackhole $net; done ;;
restore) for net in $NETS; do nsx $R ip route replace $net via 10.99.1.2; done ;;
dns-cut) nsx $R ip route replace blackhole 198.18.0.53/32 ;;
dns-restore) nsx $R ip route del blackhole 198.18.0.53/32 ;;
gw-ping) nsx $R sysctl -qw net.ipv4.icmp_echo_ignore_all=$([ "$2" = off ] && echo 1 || echo 0) ;;
# One target host disappears (or comes back), the rest of the net stays.
host-down) nsx $R ip route replace blackhole $2/32 ;;
host-up) nsx $R ip route del blackhole $2/32 ;;
ns-exec) shift; nsx $NS sudo -u "$(id -un)" "$@" ;;
*) sed -n '2,15p' "$0"; exit 2 ;;
esac
