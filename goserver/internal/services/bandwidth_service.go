package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/LevanPro/server/internal/models"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const (
	openVPNStatusFile  = "/var/log/openvpn/status.log"
	ipsecContainerName = "ipsec-mobify-server"
	accumulatorFile    = "accumulator.json"
)

type BandwidthService struct {
	dockerClient       *client.Client
	storagePath        string
	collectionInterval time.Duration
	logger             *slog.Logger

	// Tracking state
	accumulator    *models.BandwidthAccumulator
	lastIPSecBytes uint64
	mu             sync.RWMutex

	// Lifecycle
	ticker *time.Ticker
	done   chan struct{}
	wg     sync.WaitGroup
}

func NewBandwidthService(storagePath string, collectionInterval time.Duration, logger *slog.Logger) (*BandwidthService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	s := &BandwidthService{
		dockerClient:       cli,
		storagePath:        storagePath,
		collectionInterval: collectionInterval,
		logger:             logger,
		done:               make(chan struct{}),
	}

	// Try to load existing accumulator, or initialize new one
	if err := s.loadAccumulator(); err != nil {
		logger.Warn("Failed to load accumulator, initializing new one", "error", err.Error())
		s.accumulator = &models.BandwidthAccumulator{
			LastResetAt:  time.Now().UTC(),
			LastUpdated:  time.Now().UTC(),
			ClientStates: make(map[string]models.ClientState),
		}
	}

	return s, nil
}

// Start launches the background tracking goroutine
func (s *BandwidthService) Start() error {
	s.ticker = time.NewTicker(s.collectionInterval)
	s.wg.Add(1)
	go s.trackingLoop()
	s.logger.Info("Bandwidth tracking started", "interval", s.collectionInterval)
	return nil
}

// trackingLoop runs the periodic bandwidth collection
func (s *BandwidthService) trackingLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ticker.C:
			if err := s.collectAndAccumulate(); err != nil {
				s.logger.Error("Failed to collect and accumulate bandwidth", "error", err.Error())
			}
		case <-s.done:
			s.logger.Info("Bandwidth tracking stopped")
			return
		}
	}
}

// collectAndAccumulate collects current metrics and updates the accumulator
func (s *BandwidthService) collectAndAccumulate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse current OpenVPN clients
	currentClients, err := s.parseOpenVPNClients()
	if err != nil {
		s.logger.Warn("Failed to parse OpenVPN clients", "error", err.Error())
		currentClients = make(map[string]models.ClientState)
	}

	// Calculate OpenVPN deltas
	previousClients := s.accumulator.ClientStates
	s.calculateOpenVPNDeltas(currentClients, previousClients)

	// Update client states
	s.accumulator.ClientStates = currentClients

	// Collect IPSec metrics
	ipsecBytes, err := s.collectIPSecBytes()
	if err != nil {
		s.logger.Warn("Failed to collect IPSec metrics", "error", err.Error())
		ipsecBytes = 0
	}

	// Calculate IPSec delta
	if s.lastIPSecBytes > 0 && ipsecBytes >= s.lastIPSecBytes {
		delta := ipsecBytes - s.lastIPSecBytes
		s.accumulator.IPSec.TotalBytesSent += delta / 2      // Approximate split
		s.accumulator.IPSec.TotalBytesReceived += delta / 2
	}
	s.lastIPSecBytes = ipsecBytes

	// Update totals
	s.accumulator.IPSec.TotalBandwidthMB = float64(s.accumulator.IPSec.TotalBytesSent+s.accumulator.IPSec.TotalBytesReceived) / (1024 * 1024)
	s.accumulator.OpenVPN.TotalBandwidthMB = float64(s.accumulator.OpenVPN.TotalBytesSent+s.accumulator.OpenVPN.TotalBytesReceived) / (1024 * 1024)
	s.accumulator.LastUpdated = time.Now().UTC()

	// Persist to disk
	if err := s.saveAccumulator(); err != nil {
		return fmt.Errorf("failed to save accumulator: %w", err)
	}

	return nil
}

// parseOpenVPNClients parses the OpenVPN status file and returns current client states
func (s *BandwidthService) parseOpenVPNClients() (map[string]models.ClientState, error) {
	clients := make(map[string]models.ClientState)

	file, err := os.Open(openVPNStatusFile)
	if err != nil {
		if os.IsNotExist(err) {
			return clients, nil
		}
		return nil, fmt.Errorf("failed to open OpenVPN status file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse CLIENT_LIST lines
		// Format: CLIENT_LIST,common_name,real_address,virtual_ip,virtual_ipv6,bytes_received,bytes_sent,connected_since,username,client_id,peer_id
		if strings.HasPrefix(line, "CLIENT_LIST") {
			fields := strings.Split(line, ",")
			if len(fields) >= 8 {
				commonName := fields[1]
				realAddress := fields[2]
				bytesReceived, err1 := strconv.ParseUint(fields[5], 10, 64)
				bytesSent, err2 := strconv.ParseUint(fields[6], 10, 64)
				connectedSince, err3 := time.Parse("2006-01-02 15:04:05", fields[7])

				if err1 == nil && err2 == nil {
					if err3 != nil {
						connectedSince = time.Now().UTC()
					}

					clients[commonName] = models.ClientState{
						CommonName:     commonName,
						RealAddress:    realAddress,
						BytesSent:      bytesSent,
						BytesReceived:  bytesReceived,
						ConnectedSince: connectedSince,
						LastSeenAt:     time.Now().UTC(),
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading OpenVPN status file: %w", err)
	}

	return clients, nil
}

// calculateOpenVPNDeltas calculates bandwidth deltas and updates the accumulator
func (s *BandwidthService) calculateOpenVPNDeltas(current, previous map[string]models.ClientState) {
	// Track deltas for existing clients
	for commonName, currentState := range current {
		if prevState, exists := previous[commonName]; exists {
			// Client continued session - calculate delta
			deltaSent := int64(currentState.BytesSent) - int64(prevState.BytesSent)
			deltaReceived := int64(currentState.BytesReceived) - int64(prevState.BytesReceived)

			// Handle counter rollover (treat as new connection)
			if deltaSent < 0 {
				deltaSent = int64(currentState.BytesSent)
			}
			if deltaReceived < 0 {
				deltaReceived = int64(currentState.BytesReceived)
			}

			s.accumulator.OpenVPN.TotalBytesSent += uint64(deltaSent)
			s.accumulator.OpenVPN.TotalBytesReceived += uint64(deltaReceived)
		}
		// New clients: wait for next poll to get delta
	}

	// Capture final bandwidth for disconnected clients
	for commonName, prevState := range previous {
		if _, stillConnected := current[commonName]; !stillConnected {
			// Client disconnected - capture final contribution
			s.accumulator.OpenVPN.TotalBytesSent += prevState.BytesSent
			s.accumulator.OpenVPN.TotalBytesReceived += prevState.BytesReceived
			s.accumulator.OpenVPN.SessionCount++
		}
	}
}

// collectIPSecBytes returns total network bytes for IPSec container
func (s *BandwidthService) collectIPSecBytes() (uint64, error) {
	ctx := context.Background()

	stats, err := s.dockerClient.ContainerStats(ctx, ipsecContainerName, false)
	if err != nil {
		return 0, nil // Container not running
	}
	defer stats.Body.Close()

	var containerStats container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&containerStats); err != nil {
		return 0, fmt.Errorf("failed to decode container stats: %w", err)
	}

	var totalBytes uint64
	for _, network := range containerStats.Networks {
		totalBytes += network.RxBytes + network.TxBytes
	}

	return totalBytes, nil
}

// loadAccumulator loads the accumulator from disk
func (s *BandwidthService) loadAccumulator() error {
	filePath := filepath.Join(s.storagePath, accumulatorFile)

	// Try to open with shared lock
	var file *os.File
	var err error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		file, err = os.OpenFile(filePath, os.O_RDONLY, 0)
		if err != nil {
			if os.IsNotExist(err) {
				return err
			}
			if i == maxRetries-1 {
				return fmt.Errorf("failed to open accumulator file: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		err = syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err != nil {
			file.Close()
			if i == maxRetries-1 {
				return fmt.Errorf("failed to acquire lock: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	defer file.Close()

	var acc models.BandwidthAccumulator
	if err := json.NewDecoder(file).Decode(&acc); err != nil {
		return fmt.Errorf("failed to decode accumulator: %w", err)
	}

	if acc.ClientStates == nil {
		acc.ClientStates = make(map[string]models.ClientState)
	}

	s.accumulator = &acc
	return nil
}

// saveAccumulator persists the accumulator to disk
func (s *BandwidthService) saveAccumulator() error {
	filePath := filepath.Join(s.storagePath, accumulatorFile)

	// Open with exclusive lock
	var file *os.File
	var err error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		file, err = os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("failed to open accumulator file: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err != nil {
			file.Close()
			if i == maxRetries-1 {
				return fmt.Errorf("failed to acquire lock: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.accumulator); err != nil {
		return fmt.Errorf("failed to encode accumulator: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// GetAccumulatedMetrics returns the accumulated bandwidth metrics
func (s *BandwidthService) GetAccumulatedMetrics() (*models.BandwidthMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	combinedTotalMB := s.accumulator.OpenVPN.TotalBandwidthMB + s.accumulator.IPSec.TotalBandwidthMB

	return &models.BandwidthMetrics{
		Timestamp: s.accumulator.LastUpdated,
		OpenVPN: models.OpenVPNMetrics{
			TotalBytesSent:     s.accumulator.OpenVPN.TotalBytesSent,
			TotalBytesReceived: s.accumulator.OpenVPN.TotalBytesReceived,
			TotalBandwidthMB:   s.accumulator.OpenVPN.TotalBandwidthMB,
			ActiveClients:      len(s.accumulator.ClientStates),
		},
		IPSec: models.IPSecMetrics{
			TotalBytesSent:     s.accumulator.IPSec.TotalBytesSent,
			TotalBytesReceived: s.accumulator.IPSec.TotalBytesReceived,
			TotalBandwidthMB:   s.accumulator.IPSec.TotalBandwidthMB,
		},
		CombinedTotalMB: combinedTotalMB,
	}, nil
}

// ResetAccumulator resets the bandwidth accumulator to zero
func (s *BandwidthService) ResetAccumulator() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.accumulator = &models.BandwidthAccumulator{
		LastResetAt:  now,
		LastUpdated:  now,
		ClientStates: make(map[string]models.ClientState),
	}
	s.lastIPSecBytes = 0

	if err := s.saveAccumulator(); err != nil {
		return fmt.Errorf("failed to save reset accumulator: %w", err)
	}

	s.logger.Info("Bandwidth accumulator reset")
	return nil
}

// GetMetrics collects and returns aggregated bandwidth metrics from both OpenVPN and IPSec
// This is the original snapshot method, kept for backward compatibility
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

// Close stops the background tracking and closes the Docker client connection
func (s *BandwidthService) Close() error {
	// Stop the ticker if running
	if s.ticker != nil {
		s.ticker.Stop()
	}

	// Signal the goroutine to stop
	close(s.done)

	// Wait for goroutine to finish
	s.wg.Wait()

	// Close Docker client
	if s.dockerClient != nil {
		return s.dockerClient.Close()
	}
	return nil
}
