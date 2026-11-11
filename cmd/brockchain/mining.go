package main

import (
	"context"
	"errors"
	"fmt"

	"brockchain/internal/chain"
	"brockchain/internal/protocol"
	"brockchain/internal/wallet"
)

func miningDemo() error {
	alice, err := wallet.New()
	if err != nil {
		return err
	}
	bob, err := wallet.New()
	if err != nil {
		return err
	}
	miner, err := wallet.New()
	if err != nil {
		return err
	}
	genesis := protocol.Genesis{
		Authority:   alice.Address(),
		Allocations: []protocol.Allocation{{Address: alice.Address(), Balance: 100}},
	}
	node, err := chain.New(genesis)
	if err != nil {
		return err
	}
	tx, err := alice.Transfer(node.ChainID(), bob.Address(), 3, 0, "Thanks for lunch!")
	if err != nil {
		return err
	}
	candidate, err := node.Candidate(miner.Address(), []protocol.Transfer{tx}, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Before mining: Alice=%d, Bob=%d, Miner=%d\n",
		node.Account(alice.Address()).Balance, node.Account(bob.Address()).Balance,
		node.Account(miner.Address()).Balance)
	block, err := chain.Mine(context.Background(), candidate, node.Target())
	if err != nil {
		return err
	}
	if err := node.Append(block); err != nil {
		return err
	}
	fmt.Printf("Mined block %d: %s (nonce %d)\n", node.Height(), node.Tip(), block.Header.MiningNonce)
	raw, err := block.MarshalBinary()
	if err != nil {
		return err
	}
	replayed, err := chain.Replay(genesis, [][]byte{raw})
	if err != nil {
		return err
	}
	for _, address := range []protocol.Address{alice.Address(), bob.Address(), miner.Address()} {
		if replayed.Account(address) != node.Account(address) {
			return errors.New("replay account mismatch")
		}
	}
	if replayed.Tip() != node.Tip() {
		return errors.New("replay tip mismatch")
	}
	fmt.Printf("After validation and replay: Alice=%d (next nonce %d), Bob=%d, Miner=%d\n",
		replayed.Account(alice.Address()).Balance, replayed.Account(alice.Address()).NextNonce,
		replayed.Account(bob.Address()).Balance, replayed.Account(miner.Address()).Balance)
	return nil
}
