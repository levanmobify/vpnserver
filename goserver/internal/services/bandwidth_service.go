package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LevanPro/server/internal/models"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const (
	openVPNStatusFile  = "/var/log/openvpn/status.log"
	ipsecContainerName = "ipsec-mobify-server"
)

type BandwidthService struct {
	dockerClient *client.Client
}

func NewBandwidthService() (*BandwidthService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &BandwidthService{
		dockerClient: cli,
	}, nil
}

// GetMetrics collects and returns aggregated bandwidth metrics from both OpenVPN and IPSec
func (s *BandwidthService) GetMetrics() (*models.BandwidthMetrics, error) {
	openvpnMetrics, err := s.collectOpenVPNMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to collect OpenVPN metrics: %w", err)
	}

	ipsecMetrics, err := s.collectIPSecMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to collect IPSec metrics: %w", err)
	}

	// Calculate combined total in MB
	combinedTotalMB := openvpnMetrics.TotalBandwidthMB + ipsecMetrics.TotalBandwidthMB

	return &models.BandwidthMetrics{
		Timestamp:       time.Now().UTC(),
		OpenVPN:         *openvpnMetrics,
		IPSec:           *ipsecMetrics,
		CombinedTotalMB: combinedTotalMB,
	}, nil
}

// collectOpenVPNMetrics parses the OpenVPN status log and extracts bandwidth metrics
func (s *BandwidthService) collectOpenVPNMetrics() (*models.OpenVPNMetrics, error) {
	file, err := os.Open(openVPNStatusFile)
	if err != nil {
		// If OpenVPN is not installed or status file doesn't exist, return zeros
		if os.IsNotExist(err) {
			return &models.OpenVPNMetrics{}, nil
		}
		return nil, fmt.Errorf("failed to open OpenVPN status file: %w", err)
	}
	defer file.Close()

	var totalBytesSent uint64
	var totalBytesReceived uint64
	var activeClients int

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse CLIENT_LIST lines which contain bandwidth data
		// Format: CLIENT_LIST,client_name,real_ip:port,virtual_ip,virtual_ipv6,bytes_received,bytes_sent,connected_since,username,client_id,peer_id
		if strings.HasPrefix(line, "CLIENT_LIST") {
			fields := strings.Split(line, ",")
			if len(fields) >= 7 {
				// bytes_received is at index 5, bytes_sent is at index 6
				bytesReceived, err1 := strconv.ParseUint(fields[5], 10, 64)
				bytesSent, err2 := strconv.ParseUint(fields[6], 10, 64)

				if err1 == nil && err2 == nil {
					totalBytesReceived += bytesReceived
					totalBytesSent += bytesSent
					activeClients++
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading OpenVPN status file: %w", err)
	}

	// Convert bytes to MB
	totalBytes := totalBytesSent + totalBytesReceived
	totalBandwidthMB := float64(totalBytes) / (1024 * 1024)

	return &models.OpenVPNMetrics{
		TotalBytesSent:     totalBytesSent,
		TotalBytesReceived: totalBytesReceived,
		TotalBandwidthMB:   totalBandwidthMB,
		ActiveClients:      activeClients,
	}, nil
}

// collectIPSecMetrics uses Docker Stats API to get IPSec container network metrics
func (s *BandwidthService) collectIPSecMetrics() (*models.IPSecMetrics, error) {
	ctx := context.Background()

	// Get container stats (oneshot, not streaming)
	stats, err := s.dockerClient.ContainerStats(ctx, ipsecContainerName, false)
	if err != nil {
		// If container doesn't exist or is not running, return zeros
		return &models.IPSecMetrics{}, nil
	}
	defer stats.Body.Close()

	// Parse the stats JSON
	var containerStats container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&containerStats); err != nil {
		return nil, fmt.Errorf("failed to decode container stats: %w", err)
	}

	// Extract network metrics
	var totalBytesReceived uint64
	var totalBytesSent uint64

	// Docker provides network stats per interface
	for _, network := range containerStats.Networks {
		totalBytesReceived += network.RxBytes
		totalBytesSent += network.TxBytes
	}

	// Convert bytes to MB
	totalBytes := totalBytesSent + totalBytesReceived
	totalBandwidthMB := float64(totalBytes) / (1024 * 1024)

	return &models.IPSecMetrics{
		TotalBytesSent:     totalBytesSent,
		TotalBytesReceived: totalBytesReceived,
		TotalBandwidthMB:   totalBandwidthMB,
	}, nil
}

// Close closes the Docker client connection
func (s *BandwidthService) Close() error {
	if s.dockerClient != nil {
		return s.dockerClient.Close()
	}
	return nil
}
