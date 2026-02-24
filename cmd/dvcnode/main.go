package main

import (
	"devinsidercoin/internal/blockchain"
	"devinsidercoin/internal/config"
	"devinsidercoin/internal/network"
	"devinsidercoin/internal/rpc"
	"devinsidercoin/internal/wallet"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	networkName := flag.String("network", "mainnet", "Network: mainnet or testnet")
	dataDir := flag.String("datadir", "", "Data directory (default: ./data/<network>)")
	p2pPort := flag.Int("port", 0, "P2P port (default from config)")
	rpcPort := flag.Int("rpcport", 0, "RPC/HTTP port (default from config)")
	addPeers := flag.String("addpeer", "", "Comma-separated peer addresses (host:port)")
	configPath := flag.String("config", "", "Path to network config JSON")
	flag.Parse()

	// Find config file
	cfgPath := *configPath
	if cfgPath == "" {
		exe, _ := os.Executable()
		baseDir := filepath.Dir(exe)
		cfgPath = filepath.Join(baseDir, "networks", *networkName+".json")
		if _, err := os.Stat(cfgPath); err != nil {
			cfgPath = filepath.Join("networks", *networkName+".json")
		}
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config %s: %v", cfgPath, err)
	}

	log.Printf("=== DevInsiderCoin Node ===")
	log.Printf("Network: %s (%s)", cfg.Name, cfg.Ticker)
	log.Printf("Consensus: %s | Algorithm: %s", cfg.ConsensusType, cfg.Algorithm)

	// Data directory
	ddir := *dataDir
	if ddir == "" {
		ddir = filepath.Join("data", *networkName)
	}
	os.MkdirAll(ddir, 0755)

	// Initialize blockchain
	chain := blockchain.NewBlockchain(cfg, ddir)

	// Initialize wallet manager
	wallets := wallet.NewWalletManager(filepath.Join(ddir, "wallets"), cfg.AddressPrefix)

	// Initialize P2P node
	node := network.NewNode(cfg, chain)
	port := cfg.P2PPort
	if *p2pPort > 0 {
		port = *p2pPort
	}
	if err := node.Start(port); err != nil {
		log.Fatalf("Failed to start P2P: %v", err)
	}

	// Connect to peers
	if *addPeers != "" {
		for _, addr := range strings.Split(*addPeers, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			log.Printf("[P2P] Connecting to peer: %s", addr)
			if err := node.ConnectPeer(addr); err != nil {
				log.Printf("[P2P] Failed to connect to %s: %v", addr, err)
			}
		}
	}

	// Start RPC/HTTP server
	rPort := cfg.RPCPort
	if *rpcPort > 0 {
		rPort = *rpcPort
	}
	srv := &rpc.Server{
		Chain:   chain,
		Node:    node,
		Wallets: wallets,
		Addr:    fmt.Sprintf(":%d", rPort),
	}
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("RPC server error: %v", err)
		}
	}()

	log.Printf("=== Node running ===")
	log.Printf("  P2P:  :%d", port)
	log.Printf("  RPC:  http://localhost:%d/rpc", rPort)
	log.Printf("  API:  http://localhost:%d/api/", rPort)
	log.Printf("  Data: %s", ddir)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
}
