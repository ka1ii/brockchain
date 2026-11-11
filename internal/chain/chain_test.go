package chain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"brockchain/internal/protocol"
)

type vectors struct {
	Genesis map[string]struct {
		Configuration string `json:"configuration_hex"`
	}
	Blocks map[string]struct {
		Raw  string `json:"full_hex"`
		Hash string `json:"block_hash"`
	}
	Updates map[string]struct {
		Raw     string `json:"full_hex"`
		Message string `json:"signing_message_hex"`
	}
	Transfers map[string]struct {
		Raw string `json:"full_hex"`
	}
	Invalid []struct {
		Name string
		Kind string
		Raw  string `json:"full_hex"`
	}
}

func loadVectors(t *testing.T) (vectors, protocol.Genesis) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/protocol-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	g, err := protocol.DecodeGenesis(unhex(t, v.Genesis["funded"].Configuration))
	if err != nil {
		t.Fatal(err)
	}
	return v, g
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestBlockAndUpdateFixtures(t *testing.T) {
	v, g := loadVectors(t)
	for name, fixture := range v.Blocks {
		t.Run(name, func(t *testing.T) {
			raw := unhex(t, fixture.Raw)
			b, err := protocol.DecodeBlock(raw)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := b.MarshalBinary()
			if err != nil || !bytes.Equal(encoded, raw) || b.Header.Hash().String() != fixture.Hash {
				t.Fatal("block fixture mismatch", err)
			}
			c, err := Replay(g, [][]byte{raw})
			if err != nil || c.Height() != 1 || c.Tip() != b.Header.Hash() {
				t.Fatal("fixture replay failed", err)
			}
			if name == "populated" {
				tx := b.Transfers[0]
				if c.Account(tx.Sender).Balance != 97 || c.Account(tx.Sender).NextNonce != 1 ||
					c.Account(tx.Recipient).Balance != 78 || c.Account(tx.Recipient).NextNonce != 0 {
					t.Fatal("transfer or reward balances incorrect")
				}
				if c.Target() != b.Update.NewTarget || c.sequence != 1 {
					t.Fatal("updated target missing after replay")
				}
			}
		})
	}
	for name, fixture := range v.Updates {
		t.Run(name, func(t *testing.T) {
			raw := unhex(t, fixture.Raw)
			u, err := protocol.DecodeTargetUpdate(raw)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := u.MarshalBinary()
			if err != nil || !bytes.Equal(encoded, raw) {
				t.Fatal("target update encoding mismatch", err)
			}
			msg, err := u.SigningMessage()
			if err != nil || hex.EncodeToString(msg) != fixture.Message {
				t.Fatal("target update message mismatch", err)
			}
			if err := u.Verify(g.Authority); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, fixture := range v.Invalid {
		switch fixture.Kind {
		case "block":
			t.Run(fixture.Name, func(t *testing.T) {
				if _, err := Replay(g, [][]byte{unhex(t, fixture.Raw)}); err == nil {
					t.Fatal("accepted invalid block fixture")
				}
			})
		case "update":
			t.Run(fixture.Name, func(t *testing.T) {
				u, err := protocol.DecodeTargetUpdate(unhex(t, fixture.Raw))
				if err == nil && u.Verify(g.Authority) == nil {
					t.Fatal("accepted invalid update fixture")
				}
			})
		}
	}
}

func TestMiningValidationAndReplay(t *testing.T) {
	v, g := loadVectors(t)
	c, err := New(g)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := protocol.DecodeTransfer(unhex(t, v.Transfers["ascii"].Raw))
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := protocol.DecodeTransfer(unhex(t, v.Transfers["u64_max_fields"].Raw))
	if err != nil {
		t.Fatal(err)
	}
	u, err := protocol.DecodeTargetUpdate(unhex(t, v.Updates["maximum_target"].Raw))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := c.Candidate(tx.Recipient, []protocol.Transfer{invalid, tx, tx}, &u)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Transfers) != 1 || c.Account(tx.Sender).Balance != 100 {
		t.Fatal("candidate should select one executable transfer without changing balances")
	}

	// Recommit an invalid second transfer and mine it so validation reaches execution.
	bad := candidate
	bad.Transfers = []protocol.Transfer{tx, invalid}
	bad.Header.TransactionsHash, bad.Header.TargetUpdateHash, err = bad.Commitments()
	if err != nil {
		t.Fatal(err)
	}
	bad, err = Mine(context.Background(), bad, c.Target())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Append(bad); err == nil || !strings.Contains(err.Error(), "transfer 1:") {
		t.Fatalf("expected failure on second transfer, got %v", err)
	}
	if c.Height() != 0 || c.Tip() != c.ChainID() || c.Target() != protocol.InitialTarget() ||
		c.sequence != 0 || c.supply != 125 || c.Account(tx.Sender).Balance != 100 ||
		c.Account(tx.Recipient).Balance != 25 || c.Account(tx.Sender).NextNonce != 0 {
		t.Fatal("invalid block changed chain state")
	}

	first, err := Mine(context.Background(), candidate, c.Target())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Append(first); err != nil {
		t.Fatal(err)
	}
	child, err := c.Candidate(tx.Recipient, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Pick a nonce that would fail under the old target. At the new maximum
	// target every nonce is valid, so mining must return this nonce immediately.
	for protocol.MeetsTarget(child.Header.Hash(), protocol.InitialTarget()) {
		child.Header.MiningNonce++
	}
	second, err := Mine(context.Background(), child, c.Target())
	if err != nil || second.Header.MiningNonce != child.Header.MiningNonce {
		t.Fatal("child did not use the new target", err)
	}
	if err := c.Append(second); err != nil {
		t.Fatal(err)
	}
	raw := make([][]byte, 0, 2)
	for _, b := range []protocol.Block{first, second} {
		data, err := b.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, data)
	}
	replayed, err := Replay(g, raw)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Tip() != c.Tip() || replayed.Target() != u.NewTarget || replayed.supply != 225 ||
		replayed.Account(tx.Sender) != c.Account(tx.Sender) || replayed.Account(tx.Recipient).Balance != 128 {
		t.Fatal("replay did not reconstruct balances, reward supply, target, or tip")
	}
}

func TestMiningCancellationAndTargetBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Mine(ctx, protocol.Block{}, protocol.InitialTarget()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	target := protocol.Hash{31: 2}
	if !protocol.MeetsTarget(target, target) || protocol.MeetsTarget(protocol.Hash{31: 3}, target) {
		t.Fatal("incorrect inclusive target comparison")
	}
}
