package state

import (
	"brockchain/internal/protocol"
	"crypto/ed25519"
	"math"
	"reflect"
	"testing"
)

func setup(t *testing.T) (*State, ed25519.PrivateKey, protocol.Address, protocol.Address) {
	t.Helper()
	seed := make([]byte, 32)
	seed[0] = 1
	key := ed25519.NewKeyFromSeed(seed)
	var a, b protocol.Address
	copy(a[:], key.Public().(ed25519.PublicKey))
	b[0] = 2
	s, e := New(protocol.Genesis{Allocations: []protocol.Allocation{{Address: a, Balance: 100}}})
	if e != nil {
		t.Fatal(e)
	}
	return s, key, a, b
}
func sign(t *testing.T, s *State, key ed25519.PrivateKey, a, b protocol.Address, amount, nonce uint64) protocol.Transfer {
	t.Helper()
	tx := protocol.Transfer{Version: 1, ChainID: s.ChainID(), Sender: a, Recipient: b, Amount: amount, Nonce: nonce, Memo: "hello"}
	if e := tx.Sign(key); e != nil {
		t.Fatal(e)
	}
	return tx
}
func TestTransferAndReplay(t *testing.T) {
	s, key, a, b := setup(t)
	tx := sign(t, s, key, a, b, 3, 0)
	if e := s.Apply(tx); e != nil {
		t.Fatal(e)
	}
	if s.Account(a) != (Account{97, 1}) || s.Account(b) != (Account{3, 0}) {
		t.Fatal("incorrect balances/nonces")
	}
	before := s.Clone()
	if e := s.Apply(tx); e == nil {
		t.Fatal("accepted replay")
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("replay changed state")
	}
	tx = sign(t, s, key, a, b, 97, 1)
	if e := s.Apply(tx); e != nil {
		t.Fatal(e)
	}
	if s.Account(a) != (Account{0, 2}) || s.Account(b).Balance != 100 {
		t.Fatal("exact balance spend failed")
	}
}
func TestRejectionsAreAtomic(t *testing.T) {
	for _, name := range []string{"overspend", "future_nonce", "wrong_chain", "bad_signature", "memo_tamper", "zero", "self", "balance_overflow", "nonce_overflow", "bad_utf8", "long_memo"} {
		t.Run(name, func(t *testing.T) {
			s, key, a, b := setup(t)
			tx := sign(t, s, key, a, b, 3, 0)
			switch name {
			case "overspend":
				tx.Amount = 101
			case "future_nonce":
				tx.Nonce = 1
			case "wrong_chain":
				tx.ChainID[0] ^= 1
			case "zero":
				tx.Amount = 0
			case "self":
				tx.Recipient = a
			case "balance_overflow":
				s.accounts[b] = Account{Balance: math.MaxUint64}
			case "nonce_overflow":
				s.accounts[a] = Account{Balance: 100, NextNonce: math.MaxUint64}
				tx.Nonce = math.MaxUint64
			case "bad_utf8":
				tx.Memo = string([]byte{255})
			case "long_memo":
				tx.Memo = string(make([]byte, 257))
			}
			if name != "bad_utf8" && name != "long_memo" {
				if e := tx.Sign(key); e != nil {
					t.Fatal(e)
				}
			}
			if name == "bad_signature" {
				tx.Signature[0] ^= 1
			}
			if name == "memo_tamper" {
				tx.Memo = "changed"
			}
			before := s.Clone()
			if e := s.Apply(tx); e == nil {
				t.Fatal("accepted invalid transfer")
			}
			if !reflect.DeepEqual(s, before) {
				t.Fatal("invalid transfer mutated state")
			}
		})
	}
}
func TestReceivedFundsCanBeSpentAndCloneIsIndependent(t *testing.T) {
	s, key, a, b := setup(t)
	seed := make([]byte, 32)
	seed[0] = 3
	bob := ed25519.NewKeyFromSeed(seed)
	copy(b[:], bob.Public().(ed25519.PublicKey))
	copyState := s.Clone()
	if e := s.Apply(sign(t, s, key, a, b, 10, 0)); e != nil {
		t.Fatal(e)
	}
	if copyState.Account(a).Balance != 100 || copyState.Account(b).Balance != 0 {
		t.Fatal("clone shares account map")
	}
	if e := s.Apply(sign(t, s, bob, b, a, 4, 0)); e != nil {
		t.Fatal(e)
	}
	if s.Account(a).Balance != 94 || s.Account(b) != (Account{6, 1}) {
		t.Fatal("received funds spend incorrect")
	}
}
