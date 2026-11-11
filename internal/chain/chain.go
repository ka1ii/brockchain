// Package chain validates and replays a single in-memory chain.
package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"brockchain/internal/protocol"
	"brockchain/internal/state"
)

type Chain struct {
	state     *state.State
	authority protocol.Address
	tip       protocol.Hash
	height    uint64
	target    protocol.Hash
	sequence  uint64
	supply    uint64
}

func New(genesis protocol.Genesis) (*Chain, error) {
	accounts, err := state.New(genesis)
	if err != nil {
		return nil, err
	}
	c := &Chain{
		state: accounts, authority: genesis.Authority,
		tip: accounts.ChainID(), target: protocol.InitialTarget(),
	}
	for _, allocation := range genesis.Allocations {
		c.supply += allocation.Balance
	}
	return c, nil
}

func (c *Chain) ChainID() protocol.Hash                         { return c.state.ChainID() }
func (c *Chain) Tip() protocol.Hash                             { return c.tip }
func (c *Chain) Height() uint64                                 { return c.height }
func (c *Chain) Target() protocol.Hash                          { return c.target }
func (c *Chain) Account(address protocol.Address) state.Account { return c.state.Account(address) }

func (c *Chain) validateUpdate(update *protocol.TargetUpdate) error {
	if update == nil {
		return nil
	}
	if update.ChainID != c.ChainID() || update.ParentHash != c.tip {
		return errors.New("target update belongs to a different chain or parent")
	}
	if c.sequence == math.MaxUint64 || update.Sequence != c.sequence+1 {
		return errors.New("incorrect target update sequence")
	}
	return update.Verify(c.authority)
}

// Candidate selects executable pending transfers in transaction-ID order.
// Retrying skipped transfers allows a selected transfer to fund or unlock another.
func (c *Chain) Candidate(miner protocol.Address, pending []protocol.Transfer, update *protocol.TargetUpdate) (protocol.Block, error) {
	var block protocol.Block
	if c.height == math.MaxUint64 || c.supply > math.MaxUint64-protocol.Reward {
		return block, errors.New("cannot extend chain: height or supply limit")
	}
	if err := c.validateUpdate(update); err != nil {
		return block, err
	}
	if update != nil {
		value := *update
		block.Update = &value
	}
	type candidate struct {
		id protocol.Hash
		tx protocol.Transfer
	}
	ordered := make([]candidate, 0, len(pending))
	seen := make(map[protocol.Hash]bool, len(pending))
	for _, tx := range pending {
		id, err := tx.ID()
		if err == nil && !seen[id] && tx.Verify(c.ChainID()) == nil {
			seen[id] = true
			ordered = append(ordered, candidate{id, tx})
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i].id[:], ordered[j].id[:]) < 0 })
	scratch := c.state.Clone()
	for len(ordered) > 0 && len(block.Transfers) < protocol.MaxTransfers {
		before := len(block.Transfers)
		skipped := ordered[:0]
		for _, entry := range ordered {
			if scratch.Apply(entry.tx) != nil {
				skipped = append(skipped, entry)
				continue
			}
			block.Transfers = append(block.Transfers, entry.tx)
			if len(block.Transfers) == protocol.MaxTransfers {
				break
			}
		}
		if len(block.Transfers) == before {
			break
		}
		ordered = skipped
	}
	txHash, updateHash, err := block.Commitments()
	if err != nil {
		return protocol.Block{}, err
	}
	block.Header = protocol.Header{
		Version: protocol.Version, ChainID: c.ChainID(), ParentHash: c.tip,
		Height: c.height + 1, Miner: miner, TransactionsHash: txHash, TargetUpdateHash: updateHash,
	}
	return block, nil
}

// Mine searches the candidate header's nonce space using the supplied parent target.
func Mine(ctx context.Context, candidate protocol.Block, target protocol.Hash) (protocol.Block, error) {
	if target == (protocol.Hash{}) {
		return protocol.Block{}, errors.New("invalid zero mining target")
	}
	for {
		if err := ctx.Err(); err != nil {
			return protocol.Block{}, err
		}
		if protocol.MeetsTarget(candidate.Header.Hash(), target) {
			return candidate, nil
		}
		if candidate.Header.MiningNonce == math.MaxUint64 {
			return protocol.Block{}, errors.New("mining nonce exhausted")
		}
		candidate.Header.MiningNonce++
	}
}

// Append checks the entire block before committing any chain or account changes.
func (c *Chain) Append(block protocol.Block) error {
	h := block.Header
	if h.Version != protocol.Version || h.ChainID != c.ChainID() {
		return errors.New("wrong block version or chain ID")
	}
	if h.ParentHash != c.tip || c.height == math.MaxUint64 || h.Height != c.height+1 {
		return errors.New("block must extend the current tip at the next height")
	}
	txHash, updateHash, err := block.Commitments()
	if err != nil {
		return err
	}
	if h.TransactionsHash != txHash || h.TargetUpdateHash != updateHash {
		return errors.New("block commitment mismatch")
	}
	if err := c.validateUpdate(block.Update); err != nil {
		return err
	}
	hash := h.Hash()
	if !protocol.MeetsTarget(hash, c.target) {
		return errors.New("invalid proof of work")
	}

	next := c.state.Clone()
	seen := make(map[protocol.Hash]bool, len(block.Transfers))
	for i, tx := range block.Transfers {
		id, err := tx.ID()
		if err != nil {
			return err
		}
		if seen[id] {
			return errors.New("duplicate transfer")
		}
		seen[id] = true
		if err := next.Apply(tx); err != nil {
			return fmt.Errorf("transfer %d: %w", i, err)
		}
	}
	if c.supply > math.MaxUint64-protocol.Reward {
		return errors.New("block reward would overflow supply")
	}
	if err := next.CreditReward(h.Miner); err != nil {
		return err
	}
	c.state, c.tip, c.height = next, hash, h.Height
	c.supply += protocol.Reward
	if block.Update != nil {
		c.target, c.sequence = block.Update.NewTarget, block.Update.Sequence
	}
	return nil
}

// Replay rebuilds the active state by decoding and validating blocks in order.
func Replay(genesis protocol.Genesis, blocks [][]byte) (*Chain, error) {
	c, err := New(genesis)
	if err != nil {
		return nil, err
	}
	for i, raw := range blocks {
		block, err := protocol.DecodeBlock(raw)
		if err != nil {
			return nil, fmt.Errorf("decode block %d: %w", i+1, err)
		}
		if err := c.Append(block); err != nil {
			return nil, fmt.Errorf("validate block %d: %w", i+1, err)
		}
	}
	return c, nil
}
