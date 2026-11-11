package protocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestReferenceVectors(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/protocol-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Chain   string `json:"chain_id"`
		Genesis map[string]struct {
			Raw   string `json:"configuration_hex"`
			Chain string `json:"chain_id"`
			Size  int    `json:"size_bytes"`
		} `json:"genesis"`
		Transfers map[string]struct {
			Raw      string `json:"full_hex"`
			Unsigned string `json:"unsigned_hex"`
			Message  string `json:"signing_message_hex"`
			ID       string `json:"transaction_id"`
			Size     int    `json:"size_bytes"`
		} `json:"transfers"`
		Keys map[string]struct {
			Seed string `json:"seed_hex"`
		} `json:"keys"`
		Invalid []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			Raw  string `json:"full_hex"`
		} `json:"invalid"`
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	chain, e := ParseHash(v.Chain)
	if e != nil {
		t.Fatal(e)
	}
	for name, r := range v.Genesis {
		t.Run("genesis/"+name, func(t *testing.T) {
			b := unhex(t, r.Raw)
			g, e := DecodeGenesis(b)
			if e != nil {
				t.Fatal(e)
			}
			encoded, e := g.MarshalBinary()
			if e != nil || !bytes.Equal(encoded, b) || len(b) != r.Size {
				t.Fatal("genesis bytes mismatch", e)
			}
			id, e := g.ChainID()
			if e != nil || id.String() != r.Chain {
				t.Fatal("chain ID mismatch", e)
			}
		})
	}
	for name, r := range v.Transfers {
		t.Run("transfer/"+name, func(t *testing.T) {
			b := unhex(t, r.Raw)
			tx, e := DecodeTransfer(b)
			if e != nil {
				t.Fatal(e)
			}
			if e = tx.Verify(chain); e != nil {
				t.Fatal(e)
			}
			encoded, e := tx.MarshalBinary()
			if e != nil || !bytes.Equal(encoded, b) || len(b) != r.Size {
				t.Fatal("transfer bytes mismatch", e)
			}
			unsigned, e := tx.UnsignedBytes()
			if e != nil || hex.EncodeToString(unsigned) != r.Unsigned {
				t.Fatal("unsigned mismatch", e)
			}
			msg, e := tx.SigningMessage()
			if e != nil || hex.EncodeToString(msg) != r.Message {
				t.Fatal("message mismatch", e)
			}
			id, e := tx.ID()
			if e != nil || id.String() != r.ID {
				t.Fatal("ID mismatch", e)
			}
			tx.Signature = [64]byte{}
			if e = tx.Sign(ed25519.NewKeyFromSeed(unhex(t, v.Keys["alice"].Seed))); e != nil {
				t.Fatal(e)
			}
			encoded, _ = tx.MarshalBinary()
			if !bytes.Equal(encoded, b) {
				t.Fatal("signature differs from reference")
			}
		})
	}
	for _, r := range v.Invalid {
		if r.Kind != "genesis" && r.Kind != "transfer" {
			continue
		}
		t.Run("reject/"+r.Name, func(t *testing.T) {
			var err error
			if r.Kind == "genesis" {
				_, err = DecodeGenesis(unhex(t, r.Raw))
			} else {
				var tx Transfer
				tx, err = DecodeTransfer(unhex(t, r.Raw))
				if err == nil {
					err = tx.Verify(chain)
				}
			}
			if err == nil {
				t.Fatal("accepted invalid fixture")
			}
		})
	}
}
func TestDecodeTransferTruncations(t *testing.T) {
	tx := Transfer{Version: Version, Memo: "🌍"}
	b, e := tx.MarshalBinary()
	if e != nil {
		t.Fatal(e)
	}
	for n := 0; n < len(b); n++ {
		if _, e := DecodeTransfer(b[:n]); e == nil {
			t.Fatalf("accepted prefix length %d", n)
		}
	}
}
