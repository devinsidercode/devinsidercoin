package network

import (
	"bufio"
	"devinsidercoin/internal/blockchain"
	"devinsidercoin/internal/config"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Message is the P2P wire format.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// VersionPayload is sent during handshake.
type VersionPayload struct {
	Version   uint32 `json:"version"`
	Height    uint64 `json:"height"`
	NetworkID uint32 `json:"network_id"`
}

// GetBlocksPayload requests blocks from a height.
type GetBlocksPayload struct {
	FromHeight uint64 `json:"from_height"`
}

// Peer represents a connected peer.
type Peer struct {
	Conn    net.Conn
	Address string
	Height  uint64
	writer  *bufio.Writer
	mu      sync.Mutex
}

func (p *Peer) Send(msg Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = p.writer.WriteString(string(data) + "\n")
	if err != nil {
		return err
	}
	return p.writer.Flush()
}

// Node is the P2P networking layer.
type Node struct {
	Config     *config.NetworkConfig
	Chain      *blockchain.Blockchain
	Peers      map[string]*Peer
	listener   net.Listener
	mu         sync.RWMutex
	OnNewBlock func(*blockchain.Block)
}

// NewNode creates a P2P node.
func NewNode(cfg *config.NetworkConfig, chain *blockchain.Blockchain) *Node {
	return &Node{
		Config: cfg,
		Chain:  chain,
		Peers:  make(map[string]*Peer),
	}
}

// Start begins listening for P2P connections.
func (n *Node) Start(port int) error {
	var err error
	n.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	log.Printf("[P2P] Listening on :%d", port)
	go n.acceptLoop()
	return nil
}

func (n *Node) acceptLoop() {
	for {
		conn, err := n.listener.Accept()
		if err != nil {
			continue
		}
		go n.handlePeer(conn)
	}
}

// ConnectPeer connects to a remote peer.
func (n *Node) ConnectPeer(address string) error {
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return err
	}
	go n.handlePeer(conn)
	return nil
}

// GetPeerCount returns number of connected peers.
func (n *Node) GetPeerCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.Peers)
}

// GetPeerAddresses returns addresses of connected peers.
func (n *Node) GetPeerAddresses() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	addrs := make([]string, 0, len(n.Peers))
	for addr := range n.Peers {
		addrs = append(addrs, addr)
	}
	return addrs
}

// BroadcastBlock sends a block to all connected peers.
func (n *Node) BroadcastBlock(block *blockchain.Block) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	payload, _ := json.Marshal(block)
	msg := Message{Type: "block", Payload: payload}
	for _, peer := range n.Peers {
		peer.Send(msg)
	}
}

// BroadcastTx sends a transaction to all peers.
func (n *Node) BroadcastTx(tx *blockchain.Transaction) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	payload, _ := json.Marshal(tx)
	msg := Message{Type: "tx", Payload: payload}
	for _, peer := range n.Peers {
		peer.Send(msg)
	}
}

func (n *Node) handlePeer(conn net.Conn) {
	peer := &Peer{
		Conn:    conn,
		Address: conn.RemoteAddr().String(),
		writer:  bufio.NewWriter(conn),
	}

	n.mu.Lock()
	n.Peers[peer.Address] = peer
	n.mu.Unlock()

	log.Printf("[P2P] Peer connected: %s", peer.Address)

	// Send version
	vp, _ := json.Marshal(VersionPayload{
		Version:   n.Config.ProtocolVersion,
		Height:    n.Chain.GetBestHeight(),
		NetworkID: n.Config.NetworkID,
	})
	peer.Send(Message{Type: "version", Payload: vp})

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		n.handleMessage(peer, msg)
	}

	n.mu.Lock()
	delete(n.Peers, peer.Address)
	n.mu.Unlock()
	conn.Close()
	log.Printf("[P2P] Peer disconnected: %s", peer.Address)
}

func (n *Node) handleMessage(peer *Peer, msg Message) {
	switch msg.Type {
	case "version":
		var vp VersionPayload
		json.Unmarshal(msg.Payload, &vp)
		peer.Height = vp.Height
		log.Printf("[P2P] Peer %s: version=%d height=%d", peer.Address, vp.Version, vp.Height)

		ack, _ := json.Marshal(struct{}{})
		peer.Send(Message{Type: "verack", Payload: ack})

		if vp.Height > n.Chain.GetBestHeight() {
			n.requestBlocks(peer, n.Chain.GetBestHeight()+1)
		}

	case "verack":
		// Handshake complete

	case "getblocks":
		var gb GetBlocksPayload
		json.Unmarshal(msg.Payload, &gb)
		n.sendBlocks(peer, gb.FromHeight)

	case "block":
		var block blockchain.Block
		json.Unmarshal(msg.Payload, &block)
		if block.Header.Height <= n.Chain.GetBestHeight() {
			return
		}
		err := n.Chain.AddBlock(&block)
		if err != nil {
			log.Printf("[P2P] Block rejected from %s: %v", peer.Address, err)
			return
		}
		if n.OnNewBlock != nil {
			n.OnNewBlock(&block)
		}
		// Relay to other peers
		n.mu.RLock()
		payload, _ := json.Marshal(&block)
		relay := Message{Type: "block", Payload: payload}
		for addr, p := range n.Peers {
			if addr != peer.Address {
				p.Send(relay)
			}
		}
		n.mu.RUnlock()

	case "tx":
		var tx blockchain.Transaction
		json.Unmarshal(msg.Payload, &tx)
		n.Chain.AddToMempool(tx)
	}
}

func (n *Node) requestBlocks(peer *Peer, fromHeight uint64) {
	payload, _ := json.Marshal(GetBlocksPayload{FromHeight: fromHeight})
	peer.Send(Message{Type: "getblocks", Payload: payload})
}

func (n *Node) sendBlocks(peer *Peer, fromHeight uint64) {
	blocks := n.Chain.GetBlocks(fromHeight)
	for _, block := range blocks {
		payload, _ := json.Marshal(block)
		peer.Send(Message{Type: "block", Payload: payload})
	}
}
