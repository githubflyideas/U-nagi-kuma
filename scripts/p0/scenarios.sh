#!/bin/bash
# P0 scenarios on the harness (scripts/p0/harness.sh up first).
#   scenarios.sh N    runs scenario N (1-13) and prints what was sent
# BIN: the unagikuma binary. Needs sudo for the namespaces.
set -u
H=$(dirname "$0")/harness.sh
BIN=${BIN:-bin/unagikuma}
LOGS=${LOGS:-/tmp/p0}
P=19120
D=$LOGS/s$1
HOOK=$LOGS/hook.jsonl
TLOG=$LOGS/target.log
nsx() { sudo -n ip netns exec ukp0 sudo -u "$(id -un)" "$@"; }
api() { nsx curl -s -m3 "http://10.99.0.2:$P$1"; }
# Targets are steered from the internet side, so this works during a cut.
ctl() { sudo -n ip netns exec ukp0i curl -s -m3 "http://$1:8080/ctl?$2" >/dev/null; }
t0=$(date +%s)
say() { echo "[$(( $(date +%s) - t0 ))s] $*"; }

# conf SOA-TIMINGS EXTRA-LINES... : writes the configuration; monitors are
# added by the caller with mon NAME TARGET.
conf() {
  rm -rf "$D"; mkdir -p "$D"
  { echo "@ IN SOA p0. admin.localhost. ( 2026103001 $1 )"
    echo '$SET engine.first-round-delay=0s'
    shift
    for l in "$@"; do echo "$l"; done
    echo 'hook IN CHANNEL webhook url=http://192.0.2.10:19111/hook'
    echo 'default IN POLICY chain=hook'
    echo 'system IN POLICY chain=hook'; } > "$D/monitors.bind"
}
mon() { echo "$1 IN HTTP $2" >> "$D/monitors.bind"; }
sites() { for k in $(seq 0 $(( $1 - 1 ))); do mon site$k "http://198.51.$((100 + k)).10:8080/ok?i=site$k"; done; }
start() {
  nsx "$BIN" server --data "$D" --listen 10.99.0.2:$P >>"$D/server.log" 2>&1 &
  for _ in $(seq 100); do api /healthz >/dev/null && break; sleep 0.2; done
  SP=$(sudo -n ip netns pids ukp0 | head -1)
  say "server up (pid $SP)"
}
stop() { for p in $(sudo -n ip netns pids ukp0); do sudo -n kill "${1:--TERM}" "$p" 2>/dev/null; done; sleep 1; }
show() {
  python3 - "$HOOK" "$t0" <<'PY'
import json, sys, datetime
t0 = int(sys.argv[2]); n = {}
for l in open(sys.argv[1]):
    d = json.loads(l); b = d["body"]
    at = datetime.datetime.fromisoformat(d["at"]).timestamp() - t0
    k = b.get("kind", "?"); n[k] = n.get(k, 0) + 1
    print(f"  +{at:5.0f}s {k:8} {b.get('monitor') or '-':8} {b.get('class') or '':9} {b.get('title','')[:110]}")
print("  totals:", n or "no messages")
PY
}
begin() { : > "$HOOK"; : > "$TLOG"; ctl 198.51.100.10 reset=1; for k in $(seq 100 108); do ctl 198.51.$k.10 reset=1; done; ctl 203.0.113.10 reset=1; }
finish() { stop; say "messages:"; show; grep -c ' level=ERROR' "$D/server.log" | sed 's/^/  server ERROR lines: /'; }
herd() {  # requests per second at the targets from $1 to $2 (seconds since start)
  python3 - "$TLOG" "$t0" "$1" "$2" <<'PY'
import sys, datetime, collections
t0, a, b = int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4]); c = collections.Counter()
for l in open(sys.argv[1]):
    s = int(datetime.datetime.fromisoformat(l.split()[0]).timestamp()) - t0
    if a <= s < b: c[s] += 1
v = [c.get(s, 0) for s in range(a, b)]
print(f"  checks/s from +{a}s to +{b}s: max {max(v) if v else 0}, mean {sum(v)/max(1,len(v)):.1f}, first 15s: {v[:15]}")
PY
}

SHORT="10s 5s 3s 2"
begin
case $1 in
1)  say "9 monitors, default timings (60s/15s/5s/3), uplink cut for 3 minutes"
    conf "60s 15s 5s 3"; sites 9; start; sleep 75
    $H cut; say cut; sleep 180; $H restore; say restored; sleep 150; finish ;;
2)  say "9 monitors, short timings (10s/5s/3s/2), cut 3 minutes, summary on (notify-after=0s)"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 30
    $H cut; say cut; sleep 180; $H restore; say restored; sleep 120; finish ;;
3)  say "site8 really down before the outage, then a 2-minute cut"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 20
    $H host-down 198.51.108.10; say "site8 down"; sleep 40
    $H cut; say cut; sleep 120; $H restore; say restored; sleep 120
    $H host-up 198.51.108.10; finish ;;
4)  say "site7 starts failing (HTTP 500) during a 2-minute cut"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 30
    $H cut; say cut; sleep 30; ctl 198.51.107.10 "i=site7&code=500"; say "site7 → 500 (unseen: cut)"; sleep 90
    $H restore; say restored; sleep 120; finish ;;
5|13) say "1000 monitors over 9 segments, 10% failing (HTTP 500), then a 2-minute cut; herd check after it"
    conf "$SHORT" '$SET localnet.notify-after=0s'
    for i in $(seq 0 999); do mon m$i "http://198.51.$((100 + i % 9)).10:8080/ok?i=m$i"; done
    for i in $(seq 0 10 999); do ctl 198.51.$((100 + i % 9)).10 "i=m$i&code=500"; done
    start; sleep 60; say "steady"; herd 40 60
    $H cut; say cut; sleep 120; $H restore; r=$(( $(date +%s) - t0 )); say restored; sleep 120
    herd $r $((r + 40)); finish ;;
6)  say "kill -9 in the middle of a 3-minute cut, restarted during it"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 30
    $H cut; say cut; sleep 60; stop -KILL; say "killed -9"; sleep 10; start; sleep 110
    $H restore; say restored; sleep 120; finish ;;
7)  say "only 3 monitors (3 segments), 2-minute cut"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 3; start; sleep 30
    $H cut; say cut; sleep 120; $H restore; say restored; sleep 120; finish ;;
8)  say "flapping uplink: 5 × (25s down, 20s up)"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 30
    for k in 1 2 3 4 5; do $H cut; say "cut $k"; sleep 25; $H restore; say "up $k"; sleep 20; done
    sleep 120; finish ;;
9)  say "gateway does not answer ping; 2-minute cut"
    $H gw-ping off
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 40
    $H cut; say cut; sleep 120; $H restore; say restored; sleep 120; $H gw-ping on; finish ;;
10) say "DNS-only failure: the resolver is unreachable for 2 minutes; 6 monitors by name, 9 by address"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9
    for k in 1 2 3 4 5 6; do mon name$k "http://svc$k.p0.test:8080/ok?i=name$k"; done
    start; sleep 30; $H dns-cut; say "dns cut"; sleep 120; $H dns-restore; say "dns back"; sleep 120; finish ;;
11) say "SIGSTOP for 90s (a suspended host), then SIGCONT"
    conf "$SHORT" '$SET localnet.notify-after=0s'; sites 9; start; sleep 30
    stop -STOP; say stopped; sleep 90; stop -CONT; say continued; sleep 90; finish ;;
12) say "max-hold watchdog: localnet.max-hold=1m, cut 3 minutes"
    conf "$SHORT" '$SET localnet.notify-after=0s' '$SET localnet.max-hold=1m'; sites 9; start; sleep 30
    $H cut; say cut; sleep 180; $H restore; say restored; sleep 120; finish ;;
*)  sed -n '2,5p' "$0"; exit 2 ;;
esac
