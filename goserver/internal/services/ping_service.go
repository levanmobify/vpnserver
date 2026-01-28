package services

import (
	"fmt"
	"log/slog"
	"net"
)

type PingService struct {
	address string
	conn    *net.UDPConn
	logger  *slog.Logger
}

func NewPingService(address string, logger *slog.Logger) (*PingService, error) {
	return &PingService{
		address: address,
		logger:  logger,
	}, nil
}

func (ps *PingService) Start() error {
	addr, err := net.ResolveUDPAddr("udp", ps.address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	ps.conn = conn
	ps.logger.Info("UDP ping server started", "address", ps.address)

	go ps.serve()

	return nil
}

func (ps *PingService) serve() {
	buffer := make([]byte, 1024)

	for {
		n, clientAddr, err := ps.conn.ReadFromUDP(buffer)
		if err != nil {
			ps.logger.Error("Error reading UDP packet", "error", err)
			continue
		}

		// Echo the packet back to the sender
		_, err = ps.conn.WriteToUDP(buffer[:n], clientAddr)
		if err != nil {
			ps.logger.Error("Error sending UDP response", "error", err, "client", clientAddr)
			continue
		}

		ps.logger.Debug("Echoed UDP packet", "bytes", n, "client", clientAddr)
	}
}

func (ps *PingService) Close() error {
	if ps.conn != nil {
		ps.logger.Info("Shutting down UDP ping server")
		return ps.conn.Close()
	}
	return nil
}
