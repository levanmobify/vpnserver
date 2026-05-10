#!/usr/bin/env bash
# IPsec container abuse-mitigation rules — RETROFIT script.
#
# Sibling to openvpn-hardening.sh. The hardening rules in ipsec/run.sh only
# take effect once the lmobify/ipsec-mobify image is rebuilt with the new
# run.sh and pulled. Until then, this script applies the same rules to the
# RUNNING container via `docker exec`.
#
# After the image is rebuilt and deployed, this script is a no-op safety
# net: all inserts are idempotent.
#
# Usage:
#   sudo ./ipsec-hardening.sh apply     # insert rules into running container
#   sudo ./ipsec-hardening.sh remove    # take rules out of running container
#   sudo ./ipsec-hardening.sh status    # show currently active rules + counters
#
# Tunables (override via env): VPN_DNS_RESOLVER, VPN_UDP_PPS, VPN_UDP_BURST,
# VPN_SYN_PPS, VPN_SYN_BURST, VPN_CONN_LIMIT, VPN_DENY_UDP_PORTS,
# VPN_DENY_TCP_PORTS, IPSEC_CONTAINER, L2TP_NET, XAUTH_NET.

set -eu

CONTAINER="${IPSEC_CONTAINER:-ipsec-mobify-server}"
DNS_RESOLVER="${VPN_DNS_RESOLVER:-1.1.1.1}"
UDP_PPS="${VPN_UDP_PPS:-300}"
UDP_BURST="${VPN_UDP_BURST:-600}"
SYN_PPS="${VPN_SYN_PPS:-50}"
SYN_BURST="${VPN_SYN_BURST:-100}"
CONN_LIMIT="${VPN_CONN_LIMIT:-200}"
DENY_UDP="${VPN_DENY_UDP_PORTS:-11211,1900,19,17,389,520}"
DENY_TCP="${VPN_DENY_TCP_PORTS:-25,465,587,135,137,138,139,445}"

# Networks default to the hwdsl2/docker-ipsec-vpn-server defaults; override
# via env if the container was started with custom subnets.
L2TP_NET_DEFAULT="$(docker exec "$CONTAINER" printenv L2TP_NET 2>/dev/null || echo "192.168.42.0/24")"
XAUTH_NET_DEFAULT="$(docker exec "$CONTAINER" printenv XAUTH_NET 2>/dev/null || echo "192.168.43.0/24")"
L2TP_NET="${L2TP_NET:-$L2TP_NET_DEFAULT}"
XAUTH_NET="${XAUTH_NET:-$XAUTH_NET_DEFAULT}"

ensure_container() {
  if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    echo "ERROR: container '$CONTAINER' is not running." >&2
    exit 1
  fi
}

# Idempotent helpers — only insert if not present, only delete if present.
_dx() { docker exec "$CONTAINER" "$@"; }
ipf_ins() { _dx iptables -C FORWARD "$@" 2>/dev/null || _dx iptables -I FORWARD 1 "$@"; }
ipf_del() { while _dx iptables -C FORWARD "$@" 2>/dev/null; do _dx iptables -D FORWARD "$@"; done; }
nat_ins() { _dx iptables -t nat -C PREROUTING "$@" 2>/dev/null || _dx iptables -t nat -I PREROUTING 1 "$@"; }
nat_del() { while _dx iptables -t nat -C PREROUTING "$@" 2>/dev/null; do _dx iptables -t nat -D PREROUTING "$@"; done; }

apply_rules() {
  ensure_container

  # 1. Anti-spoof on L2TP ppp+ interfaces.
  ipf_ins -i ppp+ ! -s "$L2TP_NET" -j DROP

  # 2. DNS hijack — both protocols, both client subnets.
  for proto in udp tcp; do
    nat_ins -i ppp+ -p "$proto" --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53
    nat_ins -s "$XAUTH_NET" -p "$proto" --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53
  done

  # 3. Per-source UDP rate cap.
  ipf_ins -i ppp+ -p udp \
    -m hashlimit --hashlimit-name vpn_l2tp_udp --hashlimit-mode srcip \
    --hashlimit-above "$UDP_PPS"/sec --hashlimit-burst "$UDP_BURST" -j DROP
  ipf_ins -s "$XAUTH_NET" -p udp \
    -m hashlimit --hashlimit-name vpn_xauth_udp --hashlimit-mode srcip \
    --hashlimit-above "$UDP_PPS"/sec --hashlimit-burst "$UDP_BURST" -j DROP

  # 4. Per-source TCP SYN cap.
  ipf_ins -i ppp+ -p tcp --syn \
    -m hashlimit --hashlimit-name vpn_l2tp_syn --hashlimit-mode srcip \
    --hashlimit-above "$SYN_PPS"/sec --hashlimit-burst "$SYN_BURST" -j DROP
  ipf_ins -s "$XAUTH_NET" -p tcp --syn \
    -m hashlimit --hashlimit-name vpn_xauth_syn --hashlimit-mode srcip \
    --hashlimit-above "$SYN_PPS"/sec --hashlimit-burst "$SYN_BURST" -j DROP

  # 5. Concurrent-connection cap.
  ipf_ins -i ppp+ -m connlimit --connlimit-above "$CONN_LIMIT" --connlimit-mask 32 -j DROP
  ipf_ins -s "$XAUTH_NET" -m connlimit --connlimit-above "$CONN_LIMIT" --connlimit-mask 32 -j DROP

  # 6. Egress port denylist.
  ipf_ins -i ppp+ -p udp -m multiport --dports "$DENY_UDP" -j DROP
  ipf_ins -i ppp+ -p tcp -m multiport --dports "$DENY_TCP" -j DROP
  ipf_ins -s "$XAUTH_NET" -p udp -m multiport --dports "$DENY_UDP" -j DROP
  ipf_ins -s "$XAUTH_NET" -p tcp -m multiport --dports "$DENY_TCP" -j DROP

  echo "IPsec hardening rules applied to container '$CONTAINER'."
  echo "Note: rules are lost on container restart until image is rebuilt with the new ipsec/run.sh."
}

remove_rules() {
  ensure_container

  ipf_del -i ppp+ ! -s "$L2TP_NET" -j DROP

  for proto in udp tcp; do
    nat_del -i ppp+ -p "$proto" --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53
    nat_del -s "$XAUTH_NET" -p "$proto" --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53
  done

  ipf_del -i ppp+ -p udp \
    -m hashlimit --hashlimit-name vpn_l2tp_udp --hashlimit-mode srcip \
    --hashlimit-above "$UDP_PPS"/sec --hashlimit-burst "$UDP_BURST" -j DROP
  ipf_del -s "$XAUTH_NET" -p udp \
    -m hashlimit --hashlimit-name vpn_xauth_udp --hashlimit-mode srcip \
    --hashlimit-above "$UDP_PPS"/sec --hashlimit-burst "$UDP_BURST" -j DROP

  ipf_del -i ppp+ -p tcp --syn \
    -m hashlimit --hashlimit-name vpn_l2tp_syn --hashlimit-mode srcip \
    --hashlimit-above "$SYN_PPS"/sec --hashlimit-burst "$SYN_BURST" -j DROP
  ipf_del -s "$XAUTH_NET" -p tcp --syn \
    -m hashlimit --hashlimit-name vpn_xauth_syn --hashlimit-mode srcip \
    --hashlimit-above "$SYN_PPS"/sec --hashlimit-burst "$SYN_BURST" -j DROP

  ipf_del -i ppp+ -m connlimit --connlimit-above "$CONN_LIMIT" --connlimit-mask 32 -j DROP
  ipf_del -s "$XAUTH_NET" -m connlimit --connlimit-above "$CONN_LIMIT" --connlimit-mask 32 -j DROP

  ipf_del -i ppp+ -p udp -m multiport --dports "$DENY_UDP" -j DROP
  ipf_del -i ppp+ -p tcp -m multiport --dports "$DENY_TCP" -j DROP
  ipf_del -s "$XAUTH_NET" -p udp -m multiport --dports "$DENY_UDP" -j DROP
  ipf_del -s "$XAUTH_NET" -p tcp -m multiport --dports "$DENY_TCP" -j DROP

  echo "IPsec hardening rules removed from container '$CONTAINER'."
}

show_status() {
  ensure_container
  echo "== Container: $CONTAINER =="
  echo "L2TP_NET=$L2TP_NET   XAUTH_NET=$XAUTH_NET   DNS=$DNS_RESOLVER"
  echo
  echo "== FORWARD chain (top of chain — hardening rules sit here) =="
  _dx iptables -L FORWARD -n -v --line-numbers | head -25
  echo
  echo "== nat PREROUTING (DNS redirect rules show 'to:$DNS_RESOLVER:53') =="
  _dx iptables -t nat -L PREROUTING -n -v --line-numbers | grep -E "dpt:53|to:$DNS_RESOLVER" || echo "(no DNS redirect rules)"
}

case "${1:-}" in
  apply)  apply_rules ;;
  remove) remove_rules ;;
  status) show_status ;;
  *) echo "Usage: $0 {apply|remove|status}" >&2; exit 1 ;;
esac
