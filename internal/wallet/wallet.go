// Package wallet creates Ed25519 wallets and stores seeds separately from chain data.
package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"brockchain/internal/protocol"
)

type Wallet struct{ private ed25519.PrivateKey }

func New() (*Wallet, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Wallet{private: key}, nil
}
func (w *Wallet) Address() protocol.Address {
	var a protocol.Address
	copy(a[:], w.private.Public().(ed25519.PublicKey))
	return a
}
func (w *Wallet) Transfer(chain protocol.Hash, to protocol.Address, amount, nonce uint64, memo string) (protocol.Transfer, error) {
	t := protocol.Transfer{Version: protocol.Version, ChainID: chain, Sender: w.Address(), Recipient: to, Amount: amount, Nonce: nonce, Memo: memo}
	if amount == 0 || to == w.Address() {
		return t, errors.New("transfer needs a positive amount and different recipient")
	}
	err := t.Sign(w.private)
	return t, err
}

type diskWallet struct {
	Version uint16 `json:"version"`
	Seed    string `json:"seed_hex"`
}

// Save creates a new file with owner-only permissions and never overwrites a wallet.
// Seeds are stored unencrypted; filesystem access controls protect this learning wallet.
func (w *Wallet) Save(path string) error {
	data, err := json.MarshalIndent(diskWallet{protocol.Version, hex.EncodeToString(w.private.Seed())}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
func Load(path string) (*Wallet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d diskWallet
	if err = json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	seed, err := hex.DecodeString(d.Seed)
	if err != nil || len(seed) != ed25519.SeedSize || d.Version != protocol.Version {
		return nil, fmt.Errorf("invalid wallet format")
	}
	return &Wallet{private: ed25519.NewKeyFromSeed(seed)}, nil
}
