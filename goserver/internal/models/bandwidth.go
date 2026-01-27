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
