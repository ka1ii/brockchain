package main

import (
	"brockchain/internal/protocol"
	"brockchain/internal/state"
	"brockchain/internal/wallet"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: brockchain wallet create|address | transfer | demo | mine-demo")
	}
	switch args[0] {
	case "wallet":
		if len(args) < 2 {
			return errors.New("usage: wallet create|address --file wallet.json")
		}
		f := flag.NewFlagSet("wallet", flag.ContinueOnError)
		path := f.String("file", "wallet.json", "wallet file")
		if e := f.Parse(args[2:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("unexpected argument")
		}
		switch args[1] {
		case "create":
			w, e := wallet.New()
			if e != nil {
				return e
			}
			if e = w.Save(*path); e != nil {
				return e
			}
			fmt.Println(w.Address())
			return nil
		case "address":
			w, e := wallet.Load(*path)
			if e != nil {
				return e
			}
			fmt.Println(w.Address())
			return nil
		}
		return errors.New("unknown wallet command")
	case "transfer":
		f := flag.NewFlagSet("transfer", flag.ContinueOnError)
		path := f.String("wallet", "wallet.json", "sender wallet file")
		chainHex := f.String("chain", "", "chain ID hex")
		toHex := f.String("to", "", "recipient address hex")
		amount := f.Uint64("amount", 0, "base units")
		nonce := f.Uint64("nonce", 0, "sender's next nonce")
		memo := f.String("memo", "", "public UTF-8 memo")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return errors.New("unexpected argument")
		}
		chain, e := protocol.ParseHash(*chainHex)
		if e != nil {
			return e
		}
		to, e := protocol.ParseAddress(*toHex)
		if e != nil {
			return e
		}
		w, e := wallet.Load(*path)
		if e != nil {
			return e
		}
		t, e := w.Transfer(chain, to, *amount, *nonce, *memo)
		if e != nil {
			return e
		}
		raw, e := t.MarshalBinary()
		if e != nil {
			return e
		}
		id, e := t.ID()
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"transaction": hex.EncodeToString(raw), "transaction_id": id.String(), "status": "signed offline; not submitted or mined"})
	case "demo":
		return demo()
	case "mine-demo":
		return miningDemo()
	}
	return errors.New("unknown command")
}
func demo() error {
	alice, e := wallet.New()
	if e != nil {
		return e
	}
	bob, e := wallet.New()
	if e != nil {
		return e
	}
	g := protocol.Genesis{Authority: alice.Address(), Allocations: []protocol.Allocation{{Address: alice.Address(), Balance: 100}}}
	sort.Slice(g.Allocations, func(i, j int) bool {
		return bytes.Compare(g.Allocations[i].Address[:], g.Allocations[j].Address[:]) < 0
	})
	s, e := state.New(g)
	if e != nil {
		return e
	}
	t, e := alice.Transfer(s.ChainID(), bob.Address(), 3, 0, "Thanks for lunch!")
	if e != nil {
		return e
	}
	fmt.Println("Offline account-state demo (no blocks or mining)")
	fmt.Println("Before: Alice=100, Bob=0")
	if e = s.Apply(t); e != nil {
		return e
	}
	fmt.Printf("After signed transfer: Alice=%d (next nonce %d), Bob=%d\n", s.Account(alice.Address()).Balance, s.Account(alice.Address()).NextNonce, s.Account(bob.Address()).Balance)
	if e = s.Apply(t); e == nil {
		return errors.New("replay unexpectedly accepted")
	}
	fmt.Println("Replaying the same transfer: rejected")
	return nil
}
