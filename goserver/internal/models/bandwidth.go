package models

import "time"

// BandwidthMetrics represents the combined bandwidth metrics for all VPN services
type BandwidthMetrics struct {
	Timestamp       time.Time       `json:"timestamp"`
	OpenVPN         OpenVPNMetrics  `json:"openvpn"`
	IPSec           IPSecMetrics    `json:"ipsec"`
	CombinedTotalMB float64         `json:"combined_total_mb"`
}

// OpenVPNMetrics contains bandwidth metrics for OpenVPN
type OpenVPNMetrics struct {
	TotalBytesSent     uint64  `json:"total_bytes_sent"`
	TotalBytesReceived uint64  `json:"total_bytes_received"`
	TotalBandwidthMB   float64 `json:"total_bandwidth_mb"`
	ActiveClients      int     `json:"active_clients"`
}

// IPSecMetrics contains bandwidth metrics for IPSec
type IPSecMetrics struct {
	TotalBytesSent     uint64  `json:"total_bytes_sent"`
	TotalBytesReceived uint64  `json:"total_bytes_received"`
	TotalBandwidthMB   float64 `json:"total_bandwidth_mb"`
}

// ClientState tracks OpenVPN client bandwidth state
type ClientState struct {
	CommonName     string    `json:"common_name"`
	RealAddress    string    `json:"real_address"`
	BytesSent      uint64    `json:"bytes_sent"`
	BytesReceived  uint64    `json:"bytes_received"`
	ConnectedSince time.Time `json:"connected_since"`
	LastSeenAt     time.Time `json:"last_seen_at"`
}

// AccumulatedData stores cumulative metrics
type AccumulatedData struct {
	TotalBytesSent     uint64  `json:"total_bytes_sent"`
	TotalBytesReceived uint64  `json:"total_bytes_received"`
	TotalBandwidthMB   float64 `json:"total_bandwidth_mb"`
	SessionCount       int     `json:"session_count"`
}

// BandwidthAccumulator stores cumulative bandwidth across client sessions
type BandwidthAccumulator struct {
	LastUpdated  time.Time                `json:"last_updated"`
	LastResetAt  time.Time                `json:"last_reset_at"`
	OpenVPN      AccumulatedData          `json:"openvpn"`
	IPSec        AccumulatedData          `json:"ipsec"`
	ClientStates map[string]ClientState   `json:"client_states"` // key: common_name
}
