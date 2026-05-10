# DDoS Abuse Mitigation — Implementation Report

**Branch:** `feature/VPN-591-DDoS`
**Date:** 2026-05-10
**Status:** Code complete; production rollout pending

---

## 1. Summary

Following an abuse notice from the hosting provider, the VPN server has been hardened to prevent connected clients from launching outbound DDoS attacks. Protection layers cover both VPN stacks (IPsec and OpenVPN) and stop the specific attack pattern reported in the abuse notice (DNS query flood) along with the most common variants.

The implementation is non-breaking for legitimate users — defaults are calibrated so that normal browsing, streaming, and video calls operate well below all enforced limits.

---

## 2. The Incident

The hosting provider reported sustained outbound traffic from the server's public IP consistent with a DDoS attack:

| Detail | Value |
|---|---|
| Source | VPN server public IP (post-NAT) |
| Date / time | 2026-05-07 20:20:12 CEST |
| Pattern | UDP packets to port 53 of a third-party DNS server |
| Volume | ~10,000 packets/sec, ~4 Mbps |
| Classification | DNS flood (`ATTACK:DNS`) |

A connected VPN client was the source. Because the VPN server uses NAT for client traffic, the hosting provider sees only the server's public IP, not the underlying client.

---

## 3. Root Cause

The IPsec and OpenVPN setups previously trusted every connected client to send any traffic, at any rate, to any destination. There were:

- No per-client rate limits.
- No restrictions on outbound destination ports.
- No DNS controls (clients could flood any DNS server on the internet).
- No source-address validation (clients could spoof source IPs for amplification attacks).

This is the default behavior of both upstream stacks (`hwdsl2/docker-ipsec-vpn-server` and `hwdsl2/openvpn-install`). It is unsuitable for a multi-tenant VPN exposed to the internet.

---

## 4. Mitigation Implemented

Six rule groups now apply to every VPN-client packet, on both stacks.

### 4.1 Anti-spoofing

Drops any forwarded packet whose source IP is not from the assigned VPN subnet. Prevents the server from being used as a reflector for amplification attacks, which is the most damaging abuse pattern.

### 4.2 DNS redirect

All outbound DNS queries from VPN clients are transparently rewritten to a controlled resolver (Cloudflare `1.1.1.1` by default). Clients still receive valid DNS answers; arbitrary DNS targets cannot be reached. **This stops the exact attack pattern reported in the abuse notice.**

### 4.3 Per-client UDP packet-rate limit

Each VPN client is capped at 300 UDP packets per second. Normal browsing and video calls run at 50–200 pps, so legitimate users are unaffected. The 10,000 pps flood that caused the abuse complaint would be reduced to harmless background traffic.

### 4.4 Per-client TCP SYN-rate limit

Each VPN client is capped at 50 new TCP connections per second. Stops SYN flood attacks and aggressive port scanning.

### 4.5 Per-client concurrent-connection cap

Each VPN client is limited to 2,000 simultaneous connections. Ordinary browsing uses 30; modern mobile clients with multiple apps can reach several hundred. The cap leaves comfortable headroom for legitimate use while still catching flood patterns (which open tens of thousands).

### 4.6 Egress port denylist

Outbound traffic to a fixed list of abuse-only ports is dropped:

| Port(s) | Protocol | Reason |
|---|---|---|
| 25, 465, 587 | TCP | SMTP — spam relays |
| 135, 137, 138, 139, 445 | TCP | SMB / NetBIOS — Windows worm propagation |
| 11211 | UDP | Memcached — UDP amplifier (~51,000× factor) |
| 1900 | UDP | SSDP / UPnP — amplifier |
| 389 | UDP | CLDAP — amplifier |
| 19 | UDP | CharGen — legacy amplifier |
| 17 | UDP | QOTD — legacy amplifier |
| 520 | UDP | RIP routing — amplifier |

---

## 5. Files Changed

| File | Change | Purpose |
|---|---|---|
| `ipsec/run.sh` | +48 lines | Hardening rules added inside the container's existing iptables block. Applied automatically each container start. |
| `openvpn.sh` | +69 lines | Hardening rules added in three places: `create_firewall_rules` firewalld branch, `create_firewall_rules` iptables branch (as `ExecStart` / `ExecStop` lines on the `openvpn-iptables.service` systemd unit), and the symmetric `remove_firewall_rules` branch. |
| `openvpn-hardening.sh` | new file | Standalone retrofit script for applying the same rules to a live OpenVPN host before the next redeploy. Not used during fresh installs. |

All changes are on branch `feature/VPN-591-DDoS`.

---

## 6. Configuration

All thresholds are exposed as environment variables, so they can be tuned without changing code.

| Variable | Default | Description |
|---|---|---|
| `VPN_DNS_RESOLVER` | `1.1.1.1` | Resolver to redirect DNS queries to |
| `VPN_UDP_PPS` | `300` | UDP packets/sec per client |
| `VPN_UDP_BURST` | `600` | UDP burst tolerance |
| `VPN_SYN_PPS` | `50` | TCP SYN/sec per client |
| `VPN_SYN_BURST` | `100` | TCP SYN burst tolerance |
| `VPN_CONN_LIMIT` | `2000` | Max concurrent connections per client |
| `VPN_DENY_UDP_PORTS` | `11211,1900,19,17,389,520` | Outbound UDP ports to drop |
| `VPN_DENY_TCP_PORTS` | `25,465,587,135,137,138,139,445` | Outbound TCP ports to drop |

Defaults are deliberately conservative — they sit well above legitimate user activity but well below abusive volumes.

---

## 7. Deployment

### IPsec (Docker container)

| Mode | Effect |
|---|---|
| **Live, immediate** | Apply rules to the running container via `docker exec` — stops the active abuse without waiting for a rebuild. Rules are lost on container restart. |
| **Permanent** | Rebuild and push the `lmobify/ipsec-mobify:bw` image, then `docker compose pull && docker compose up -d`. Rules apply on every container start thereafter. |

Recommended sequence: live application today; image rebuild within the next deployment window.

### OpenVPN (host)

| Mode | Effect |
|---|---|
| **Live retrofit** | `sudo ./openvpn-hardening.sh apply && sudo ./openvpn-hardening.sh install` — applies rules to the running host and creates a systemd unit so they survive reboots. |
| **Fresh install** | The modified `openvpn.sh` already includes hardening; future reinstalls or new-server provisioning need no extra steps. |

---

## 8. Limitations

For transparency, the following abuse patterns are not fully addressed by this layer alone:

- **Slow-and-low abuse.** A client sending fewer than 300 UDP packets/sec stays under the rate limit. They cannot take a target down, but could be a nuisance. Addressed by Phase 2 (auto-disconnect).
- **IPv6 traffic.** Rules are IPv4-only. The upstream stacks support IPv6, but most abuse is IPv4 and doubling the rule set would complicate maintenance. Can be added later if the threat profile changes.
- **Attribution.** Firewall rules limit damage but don't identify *which user* is attacking. Identifying and removing abusers is Phase 2.
- **Encrypted tunnels through the VPN.** If a user runs another VPN inside this VPN, individual abuse patterns are invisible. The rate limit still caps total volume.

---

## 9. Recommended Next Steps

1. **Today** — Apply rules live to the running IPsec container and OpenVPN host. Reply to the hosting provider with a brief description of measures taken.
2. **This week** — Rebuild and push the IPsec Docker image so rules persist across container restarts.
3. **Phase 2** — Extend the Go management server (`goserver`) with auto-disconnect logic. The bandwidth tracker already polls per-client metrics every 60 seconds; adding a threshold check that disables and disconnects offending users would convert the current passive throttling into active enforcement, with email or webhook alerts.
4. **Phase 3** — Per-client outbound monitoring dashboard (pps, bps over time) so abuse becomes observable rather than only blocked. Useful both for incident review and for tuning the thresholds above.

---

## 10. Verification

After live application, the following checks confirm the rules are in effect:

- **IPsec:** `docker exec ipsec-mobify-server iptables -L FORWARD -n -v --line-numbers` should show the new DROP rules at the top of the chain, with packet counters incrementing only when abuse occurs.
- **OpenVPN:** `sudo ./openvpn-hardening.sh status` lists the active rules and counters, or `sudo iptables -L FORWARD -n -v --line-numbers | grep tun` for the raw view.
- **Functional check:** A test VPN client should still be able to browse normally, run DNS lookups (which are silently redirected), and pass speed tests — confirming legitimate usage is not impeded.
- **Abuse check:** A simulated UDP flood from a test client (e.g. `nping --udp -p 53 -c 10000 --rate 5000 <target>`) should be capped at the configured packets-per-second rate at the firewall.
