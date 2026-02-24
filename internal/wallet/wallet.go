package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mr-tron/base58"
)

// Wallet holds a keypair and derived address.
type Wallet struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// WalletManager manages multiple wallets.
type WalletManager struct {
	Wallets map[string]*Wallet `json:"wallets"`
	DataDir string             `json:"-"`
	Prefix  string             `json:"-"`
	mu      sync.RWMutex
}

// NewWalletManager creates a wallet manager.
func NewWalletManager(dataDir, prefix string) *WalletManager {
	wm := &WalletManager{
		Wallets: make(map[string]*Wallet),
		DataDir: dataDir,
		Prefix:  prefix,
	}
	wm.loadFromDisk()
	return wm
}

// CreateWallet generates a new ed25519 keypair and derives an address.
func (wm *WalletManager) CreateWallet() (*Wallet, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(pub)
	address := wm.Prefix + base58.Encode(hash[:20])

	w := &Wallet{
		Address:    address,
		PublicKey:  hex.EncodeToString(pub),
		PrivateKey: hex.EncodeToString(priv),
	}

	wm.Wallets[address] = w
	wm.saveToDisk()
	return w, nil
}

// GetWallet returns a wallet by address.
func (wm *WalletManager) GetWallet(address string) (*Wallet, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	w, ok := wm.Wallets[address]
	return w, ok
}

// ListWallets returns all wallet addresses.
func (wm *WalletManager) ListWallets() []string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	addrs := make([]string, 0, len(wm.Wallets))
	for addr := range wm.Wallets {
		addrs = append(addrs, addr)
	}
	return addrs
}

// Sign signs data with the wallet's private key.
func (wm *WalletManager) Sign(address string, data []byte) (string, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	w, ok := wm.Wallets[address]
	if !ok {
		return "", fmt.Errorf("wallet not found: %s", address)
	}
	privBytes, _ := hex.DecodeString(w.PrivateKey)
	priv := ed25519.PrivateKey(privBytes)
	sig := ed25519.Sign(priv, data)
	return hex.EncodeToString(sig), nil
}

// VerifySignature verifies an ed25519 signature.
func VerifySignature(publicKeyHex string, data []byte, signatureHex string) bool {
	pubBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pubBytes), data, sigBytes)
}

// Backup exports a wallet as JSON bytes.
func (wm *WalletManager) Backup(address string) ([]byte, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	w, ok := wm.Wallets[address]
	if !ok {
		return nil, fmt.Errorf("wallet not found: %s", address)
	}
	return json.MarshalIndent(w, "", "  ")
}

// Restore imports a wallet from JSON bytes.
func (wm *WalletManager) Restore(data []byte) (*Wallet, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	var w Wallet
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	wm.Wallets[w.Address] = &w
	wm.saveToDisk()
	return &w, nil
}

func (wm *WalletManager) saveToDisk() {
	os.MkdirAll(wm.DataDir, 0755)
	data, _ := json.MarshalIndent(wm.Wallets, "", "  ")
	os.WriteFile(filepath.Join(wm.DataDir, "wallets.json"), data, 0600)
}

func (wm *WalletManager) loadFromDisk() {
	data, err := os.ReadFile(filepath.Join(wm.DataDir, "wallets.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &wm.Wallets)
}
