#!/bin/sh
# OpenVPN host abuse-mitigation rules — RETROFIT script.
#
# As of branch feature/VPN-591-DDoS, openvpn.sh bakes these same rules into
# the openvpn-iptables.service it generates at install time. Use THIS script
# only to retrofit an OpenVPN host that was installed before that change,
# without re-running the full openvpn.sh installer.
#
# Do NOT use this script on a host where openvpn.sh was re-run with the new
# hardening logic — rules would be duplicated.
#
# Usage:
#   sudo ./openvpn-hardening.sh apply       # apply rules now (live)
#   sudo ./openvpn-hardening.sh remove      # take rules down
#   sudo ./openvpn-hardening.sh status      # show current matching rules
#   sudo ./openvpn-hardening.sh install     # install systemd unit (persists across boot)
#   sudo ./openvpn-hardening.sh uninstall   # remove systemd unit

set -eu

# ---- Tunables (override via env) ----
VPN_NET="${VPN_NET:-10.8.0.0/24}"
VPN_IFACE="${VPN_IFACE:-tun+}"
DNS_RESOLVER="${DNS_RESOLVER:-1.1.1.1}"
UDP_PPS="${UDP_PPS:-300}"
UDP_BURST="${UDP_BURST:-600}"
SYN_PPS="${SYN_PPS:-50}"
SYN_BURST="${SYN_BURST:-100}"
CONN_LIMIT="${CONN_LIMIT:-2000}"
DENY_UDP="${DENY_UDP:-11211,1900,19,17,389,520}"
DENY_TCP="${DENY_TCP:-25,465,587,135,137,138,139,445}"

UNIT_FILE="/etc/systemd/system/openvpn-hardening.service"

# Idempotent insert: only adds rule if not already present.
ins() {
  iptables -C "$@" 2>/dev/null || iptables -I "$@"
}
ins_nat() {
  iptables -t nat -C "$@" 2>/dev/null || iptables -t nat -I "$@"
}
# Idempotent delete: removes all duplicates.
del() {
  while iptables -C "$@" 2>/dev/null; do iptables -D "$@"; done
}
del_nat() {
  while iptables -t nat -C "$@" 2>/dev/null; do iptables -t nat -D "$@"; done
}

apply_rules() {
  # 1. Anti-spoof: drop forwarded packets whose source isn't a VPN-client IP.
  ins FORWARD -i "$VPN_IFACE" ! -s "$VPN_NET" -j DROP

  # 2. DNS hijack: redirect any client DNS query to the chosen resolver.
  ins_nat PREROUTING -i "$VPN_IFACE" -p udp --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53
  ins_nat PREROUTING -i "$VPN_IFACE" -p tcp --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53

  # 3. Per-source UDP rate cap (kills floods).
  ins FORWARD -i "$VPN_IFACE" -p udp \
    -m hashlimit --hashlimit-name ovpn_udp --hashlimit-mode srcip \
    --hashlimit-above "$UDP_PPS"/sec --hashlimit-burst "$UDP_BURST" -j DROP

  # 4. Per-source TCP SYN cap (kills SYN floods, scanners).
  ins FORWARD -i "$VPN_IFACE" -p tcp --syn \
    -m hashlimit --hashlimit-name ovpn_syn --hashlimit-mode srcip \
    --hashlimit-above "$SYN_PPS"/sec --hashlimit-burst "$SYN_BURST" -j DROP

  # 5. Concurrent-connection cap per client.
  ins FORWARD -i "$VPN_IFACE" \
    -m connlimit --connlimit-above "$CONN_LIMIT" --connlimit-mask 32 -j DROP

  # 6. Egress port denylist (amplifiers, mail relays, SMB worms).
  ins FORWARD -i "$VPN_IFACE" -p udp -m multiport --dports "$DENY_UDP" -j DROP
  ins FORWARD -i "$VPN_IFACE" -p tcp -m multiport --dports "$DENY_TCP" -j DROP

  echo "OpenVPN hardening rules applied."
}

remove_rules() {
  del FORWARD -i "$VPN_IFACE" ! -s "$VPN_NET" -j DROP

  del_nat PREROUTING -i "$VPN_IFACE" -p udp --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53
  del_nat PREROUTING -i "$VPN_IFACE" -p tcp --dport 53 -j DNAT --to-destination "$DNS_RESOLVER":53

  del FORWARD -i "$VPN_IFACE" -p udp \
    -m hashlimit --hashlimit-name ovpn_udp --hashlimit-mode srcip \
    --hashlimit-above "$UDP_PPS"/sec --hashlimit-burst "$UDP_BURST" -j DROP

  del FORWARD -i "$VPN_IFACE" -p tcp --syn \
    -m hashlimit --hashlimit-name ovpn_syn --hashlimit-mode srcip \
    --hashlimit-above "$SYN_PPS"/sec --hashlimit-burst "$SYN_BURST" -j DROP

  del FORWARD -i "$VPN_IFACE" \
    -m connlimit --connlimit-above "$CONN_LIMIT" --connlimit-mask 32 -j DROP

  del FORWARD -i "$VPN_IFACE" -p udp -m multiport --dports "$DENY_UDP" -j DROP
  del FORWARD -i "$VPN_IFACE" -p tcp -m multiport --dports "$DENY_TCP" -j DROP

  echo "OpenVPN hardening rules removed."
}

show_status() {
  echo "== FORWARD rules touching $VPN_IFACE =="
  iptables -L FORWARD -n -v --line-numbers | grep -E "$VPN_IFACE|tun\+|10\.8\.0" || echo "(none)"
  echo
  echo "== nat PREROUTING (DNS redirect) =="
  iptables -t nat -L PREROUTING -n -v --line-numbers | grep -E 'dpt:53|to:' || echo "(none)"
}

install_systemd() {
  script_path=$(readlink -f "$0")
  cat > "$UNIT_FILE" <<EOF
[Unit]
Description=OpenVPN host abuse-mitigation iptables rules
After=network-online.target openvpn-iptables.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=$script_path apply
ExecStop=$script_path remove

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now openvpn-hardening.service
  echo "Installed: $UNIT_FILE"
  echo "Service is enabled and running."
}

uninstall_systemd() {
  systemctl disable --now openvpn-hardening.service 2>/dev/null || true
  rm -f "$UNIT_FILE"
  systemctl daemon-reload
  echo "Removed: $UNIT_FILE"
}

case "${1:-}" in
  apply)     apply_rules ;;
  remove)    remove_rules ;;
  status)    show_status ;;
  install)   install_systemd ;;
  uninstall) uninstall_systemd ;;
  *) echo "Usage: $0 {apply|remove|status|install|uninstall}" >&2; exit 1 ;;
esac
