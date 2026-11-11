// Package state implements deterministic account transitions. It is not concurrency-safe.
package state

import (
	"brockchain/internal/protocol"
	"errors"
	"math"
)

type Account struct {
	Balance   uint64
	NextNonce uint64
}
type State struct {
	chain    protocol.Hash
	accounts map[protocol.Address]Account
}

func New(genesis protocol.Genesis) (*State, error) {
	chain, err := genesis.ChainID()
	if err != nil {
		return nil, err
	}
	s := &State{chain: chain, accounts: make(map[protocol.Address]Account)}
	for _, a := range genesis.Allocations {
		s.accounts[a.Address] = Account{Balance: a.Balance}
	}
	return s, nil
}
func (s *State) ChainID() protocol.Hash             { return s.chain }
func (s *State) Account(a protocol.Address) Account { return s.accounts[a] }
func (s *State) Clone() *State {
	c := &State{chain: s.chain, accounts: make(map[protocol.Address]Account, len(s.accounts))}
	for a, v := range s.accounts {
		c.accounts[a] = v
	}
	return c
}

// Apply validates everything before mutating either account.
func (s *State) Apply(t protocol.Transfer) error {
	if err := t.Verify(s.chain); err != nil {
		return err
	}
	if t.Amount == 0 || t.Sender == t.Recipient {
		return errors.New("positive amount and distinct accounts required")
	}
	sender, recipient := s.Account(t.Sender), s.Account(t.Recipient)
	if t.Nonce != sender.NextNonce || sender.NextNonce == math.MaxUint64 {
		return errors.New("incorrect or exhausted nonce")
	}
	if sender.Balance < t.Amount {
		return errors.New("insufficient funds")
	}
	if t.Amount > math.MaxUint64-recipient.Balance {
		return errors.New("recipient balance overflow")
	}
	sender.Balance -= t.Amount
	sender.NextNonce++
	recipient.Balance += t.Amount
	s.accounts[t.Sender] = sender
	s.accounts[t.Recipient] = recipient
	return nil
}

// CreditReward adds the fixed block reward without advancing the miner's nonce.
// The chain checks total supply and calls this only after executing transfers.
func (s *State) CreditReward(miner protocol.Address) error {
	account := s.Account(miner)
	if account.Balance > math.MaxUint64-protocol.Reward {
		return errors.New("miner balance overflow")
	}
	account.Balance += protocol.Reward
	s.accounts[miner] = account
	return nil
}
