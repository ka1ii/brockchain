// Package protocol defines the canonical v1 genesis and transfer formats.
package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

const Version uint16 = 1
const MaxMemo = 256
const MaxGenesisAccounts = 1000
const MaxTransfers = 1000
const Reward uint64 = 50

type Hash [32]byte
type Address [32]byte

func (h Hash) String() string    { return hex.EncodeToString(h[:]) }
func (a Address) String() string { return hex.EncodeToString(a[:]) }
func ParseHash(s string) (h Hash, err error) {
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 32 {
		return h, errors.New("expected 32-byte hex hash")
	}
	copy(h[:], b)
	return h, nil
}
func ParseAddress(s string) (a Address, err error) { h, e := ParseHash(s); return Address(h), e }
func Digest(domain string, b []byte) Hash {
	return sha256.Sum256(append([]byte("brockchain/"+domain+"/v1:"), b...))
}
func InitialTarget() Hash {
	var h Hash
	for i := 2; i < 32; i++ {
		h[i] = 255
	}
	return h
}
func put(b *bytes.Buffer, value any) {
	if err := binary.Write(b, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

type Allocation struct {
	Address Address
	Balance uint64
}
type Genesis struct {
	NetworkNonce Hash
	Authority    Address
	Allocations  []Allocation
}

func (g Genesis) Validate() error {
	if len(g.Allocations) > MaxGenesisAccounts {
		return errors.New("too many allocations")
	}
	var total uint64
	for i, a := range g.Allocations {
		if a.Balance == 0 {
			return errors.New("zero genesis balance")
		}
		if i > 0 && bytes.Compare(g.Allocations[i-1].Address[:], a.Address[:]) >= 0 {
			return errors.New("allocations must be unique and sorted")
		}
		if a.Balance > math.MaxUint64-total {
			return errors.New("genesis supply overflow")
		}
		total += a.Balance
	}
	return nil
}
func (g Genesis) MarshalBinary() ([]byte, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	put(&b, Version)
	b.Write(g.NetworkNonce[:])
	b.Write(g.Authority[:])
	target := InitialTarget()
	b.Write(target[:])
	put(&b, Reward)
	put(&b, uint32(MaxTransfers))
	put(&b, uint32(MaxGenesisAccounts))
	put(&b, uint32(len(g.Allocations)))
	for _, a := range g.Allocations {
		b.Write(a.Address[:])
		put(&b, a.Balance)
	}
	return b.Bytes(), nil
}
func (g Genesis) ChainID() (Hash, error) {
	b, e := g.MarshalBinary()
	if e != nil {
		return Hash{}, e
	}
	return Digest("genesis", b), nil
}
func DecodeGenesis(data []byte) (Genesis, error) {
	var g Genesis
	if len(data) < 118 {
		return g, errors.New("truncated genesis")
	}
	if binary.BigEndian.Uint16(data[:2]) != Version {
		return g, errors.New("unsupported version")
	}
	copy(g.NetworkNonce[:], data[2:34])
	copy(g.Authority[:], data[34:66])
	target := InitialTarget()
	if !bytes.Equal(data[66:98], target[:]) || binary.BigEndian.Uint64(data[98:106]) != Reward || binary.BigEndian.Uint32(data[106:110]) != MaxTransfers || binary.BigEndian.Uint32(data[110:114]) != MaxGenesisAccounts {
		return g, errors.New("invalid genesis parameters")
	}
	count := binary.BigEndian.Uint32(data[114:118])
	if count > MaxGenesisAccounts || len(data) != 118+int(count)*40 {
		return g, errors.New("invalid genesis count or length")
	}
	for offset := 118; offset < len(data); offset += 40 {
		var a Allocation
		copy(a.Address[:], data[offset:offset+32])
		a.Balance = binary.BigEndian.Uint64(data[offset+32 : offset+40])
		g.Allocations = append(g.Allocations, a)
	}
	return g, g.Validate()
}

type Transfer struct {
	Version   uint16
	ChainID   Hash
	Sender    Address
	Recipient Address
	Amount    uint64
	Nonce     uint64
	Memo      string
	Signature [64]byte
}

func (t Transfer) UnsignedBytes() ([]byte, error) {
	if t.Version != Version {
		return nil, errors.New("unsupported transfer version")
	}
	if len(t.Memo) > MaxMemo || !utf8.ValidString(t.Memo) {
		return nil, errors.New("memo must be valid UTF-8 of at most 256 bytes")
	}
	var b bytes.Buffer
	put(&b, t.Version)
	b.Write(t.ChainID[:])
	b.Write(t.Sender[:])
	b.Write(t.Recipient[:])
	put(&b, t.Amount)
	put(&b, t.Nonce)
	put(&b, uint16(len(t.Memo)))
	b.WriteString(t.Memo)
	return b.Bytes(), nil
}
func (t Transfer) SigningMessage() ([]byte, error) {
	b, e := t.UnsignedBytes()
	if e != nil {
		return nil, e
	}
	return append([]byte("brockchain/transfer/v1:"), b...), nil
}
func (t *Transfer) Sign(private ed25519.PrivateKey) error {
	if len(private) != ed25519.PrivateKeySize {
		return errors.New("invalid private key length")
	}
	if !bytes.Equal(private.Public().(ed25519.PublicKey), t.Sender[:]) {
		return errors.New("private key does not match sender")
	}
	msg, e := t.SigningMessage()
	if e != nil {
		return e
	}
	copy(t.Signature[:], ed25519.Sign(private, msg))
	return nil
}
func (t Transfer) Verify(chain Hash) error {
	if t.ChainID != chain {
		return errors.New("wrong chain ID")
	}
	msg, e := t.SigningMessage()
	if e != nil {
		return e
	}
	if !ed25519.Verify(ed25519.PublicKey(t.Sender[:]), msg, t.Signature[:]) {
		return errors.New("invalid signature")
	}
	return nil
}
func (t Transfer) MarshalBinary() ([]byte, error) {
	b, e := t.UnsignedBytes()
	if e != nil {
		return nil, e
	}
	return append(b, t.Signature[:]...), nil
}
func (t Transfer) ID() (Hash, error) {
	b, e := t.MarshalBinary()
	if e != nil {
		return Hash{}, e
	}
	return Digest("txid", b), nil
}
func DecodeTransfer(data []byte) (Transfer, error) {
	var t Transfer
	if len(data) < 180 {
		return t, errors.New("truncated transfer")
	}
	length := int(binary.BigEndian.Uint16(data[114:116]))
	if length > MaxMemo || len(data) != 180+length {
		return t, errors.New("invalid memo or transfer length")
	}
	t.Version = binary.BigEndian.Uint16(data[:2])
	copy(t.ChainID[:], data[2:34])
	copy(t.Sender[:], data[34:66])
	copy(t.Recipient[:], data[66:98])
	t.Amount = binary.BigEndian.Uint64(data[98:106])
	t.Nonce = binary.BigEndian.Uint64(data[106:114])
	t.Memo = string(data[116 : 116+length])
	copy(t.Signature[:], data[116+length:])
	if _, e := t.UnsignedBytes(); e != nil {
		return t, fmt.Errorf("decode transfer: %w", e)
	}
	return t, nil
}
