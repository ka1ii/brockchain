package wallet

import (
	"brockchain/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadAndSign(t *testing.T) {
	w, e := New()
	if e != nil {
		t.Fatal(e)
	}
	other, e := New()
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if e = w.Save(path); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("wallet readable by other users")
	}
	loaded, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if loaded.Address() != w.Address() {
		t.Fatal("changed address")
	}
	if e = w.Save(path); e == nil {
		t.Fatal("overwrote wallet")
	}
	tx, e := loaded.Transfer(protocol.Hash{}, other.Address(), 3, 0, "hello")
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Verify(protocol.Hash{}); e != nil {
		t.Fatal(e)
	}
}
func TestMalformedWallet(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	for _, data := range []string{`{}`, `{"version":1,"seed_hex":"00"}`, `{"version":2,"seed_hex":"0000000000000000000000000000000000000000000000000000000000000000"}`} {
		if e := os.WriteFile(p, []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := Load(p); e == nil {
			t.Fatal("accepted bad wallet")
		}
	}
}
