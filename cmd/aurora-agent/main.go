package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aurora-vm/aurora/internal/app/agent"
	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/network"
	"github.com/aurora-vm/aurora/internal/domain/storage"
	"github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/aurora-vm/aurora/internal/infra/incus"
	"github.com/aurora-vm/aurora/internal/infra/keystore"
	"github.com/aurora-vm/aurora/pkg/version"
)

func main() {
	vInfo := version.Get()
	log.Printf("Starting Aurora Node Agent [%s]...", vInfo.String())

	hubAddress := os.Getenv("AURORA_HUB_ADDRESS")
	if hubAddress == "" {
		hubAddress = "127.0.0.1:9443"
	}

	hubHTTPAddress := os.Getenv("AURORA_HUB_HTTP_ADDRESS")
	if hubHTTPAddress == "" {
		hubHTTPAddress = "http://127.0.0.1:8080"
	}

	nodeName := os.Getenv("AURORA_NODE_NAME")
	fqDN := os.Getenv("AURORA_NODE_FQDN")
	enrollmentToken := os.Getenv("AURORA_ENROLLMENT_TOKEN")
	stateDir := os.Getenv("AURORA_STATE_DIR")
	if stateDir == "" {
		stateDir = "/var/lib/aurora"
	}

	if err := os.MkdirAll(stateDir, 0700); err != nil {
		log.Fatalf("Failed to create node state directory %s: %v", stateDir, err)
	}

	ks, err := keystore.NewKeyStore(filepath.Join(stateDir, "tls"))
	if err != nil {
		log.Fatalf("Failed to initialize node keystore: %v", err)
	}

	driverType := os.Getenv("AURORA_DRIVER")
	incusSocket := os.Getenv("AURORA_INCUS_SOCKET")
	if incusSocket == "" {
		incusSocket = "/var/lib/incus/unix.socket"
	}

	var driver compute.HypervisorDriver
	var netDriver network.NetworkDriver
	var storageDriver storage.StorageDriver
	var consoleDriver compute.ConsoleDriver
	var imageDriver template.HypervisorImageDriver

	if driverType == "simulated" {
		log.Println("[INFO] Initializing simulated hypervisor driver (AURORA_DRIVER=simulated).")
		driver = incus.NewSimulatedDriver()
		netDriver = incus.NewSimulatedNetworkDriver()
		storageDriver = incus.NewSimulatedStorageDriver()
		consoleDriver = incus.NewSimulatedConsoleDriver()
		imageDriver = incus.NewSimulatedImageDriver()
	} else if _, err := os.Stat(incusSocket); err == nil {
		log.Printf("[INFO] Using local Incus Unix socket driver at %s", incusSocket)
		sockDriver := incus.NewSocketDriver(incusSocket)
		driver = sockDriver
		netDriver = incus.NewSocketNetworkDriver(sockDriver)
		storageDriver = incus.NewSocketStorageDriver(sockDriver)
		consoleDriver = incus.NewSocketConsoleDriver(sockDriver)
		imageDriver = incus.NewSocketImageDriver(sockDriver)
	} else {
		log.Printf("[WARN] Incus socket not found at %s. Initializing simulated hypervisor driver.", incusSocket)
		driver = incus.NewSimulatedDriver()
		netDriver = incus.NewSimulatedNetworkDriver()
		storageDriver = incus.NewSimulatedStorageDriver()
		consoleDriver = incus.NewSimulatedConsoleDriver()
		imageDriver = incus.NewSimulatedImageDriver()
	}

	daemon, err := agent.NewDaemon(agent.Config{
		HubAddress:        hubAddress,
		HubHTTPAddress:    hubHTTPAddress,
		NodeName:          nodeName,
		FQDN:              fqDN,
		EnrollmentToken:   enrollmentToken,
		HeartbeatInterval: 10 * time.Second,
		KeyStore:          ks,
		Driver:            driver,
		NetworkDriver:     netDriver,
		StorageDriver:     storageDriver,
		ConsoleDriver:     consoleDriver,
		ImageDriver:       imageDriver,
	})
	if err != nil {
		log.Fatalf("Failed to construct agent daemon: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdownSig
		log.Println("[INFO] Shutdown signal received. Stopping node agent...")
		cancel()
	}()

	if err := daemon.Run(ctx); err != nil {
		log.Fatalf("Node agent stopped with error: %v", err)
	}

	log.Println("[INFO] Node agent terminated cleanly.")
}
