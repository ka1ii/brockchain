# Brockchain

A learning blockchain built in Go. Stage 1 defines the protocol; stage 2 implements offline wallets, canonical genesis/transfer encoding, and deterministic account state. Stage 3 adds offline block assembly, mining, validation, rewards, and replay.

## Run the stage-two demo

Install Go 1.22 or later, then from this folder:

```sh
go test ./...
go run ./cmd/brockchain demo
```

The demo creates temporary in-memory wallets, funds Alice with 100 units in a demo genesis, signs a 3-unit transfer to Bob, applies it, and rejects a replay. Expected balances are Alice 97 and Bob 3; Alice's next nonce becomes 1. This exercises state transitions directly: no block is mined, no network is contacted, and no wallet files are created.

## Mine and replay a block

```sh
go run ./cmd/brockchain mine-demo
```

The demo signs a transfer from Alice to Bob, assembles it into a candidate, and searches for a valid mining nonce. Confirmed balances change when the chain validates and accepts the block. It then rebuilds the chain from genesis and the encoded block. Expected balances are Alice 97, Bob 3, and the miner 50.

The demo uses a single in-memory chain. Branch selection, disk persistence, and networking are later stages.

## Create wallets

```sh
go run ./cmd/brockchain wallet create --file alice.wallet.json
go run ./cmd/brockchain wallet create --file bob.wallet.json
go run ./cmd/brockchain wallet address --file alice.wallet.json
```

Creation prints the public address. Wallet files contain an **unencrypted private seed**, are created with owner-only permissions on Unix, and cannot be overwritten by the create command. Keep them private. These are learning wallets; OS permissions are not encryption or a backup mechanism.

## Sign an offline transfer

Use the chain ID from your shared genesis configuration and the recipient's public address:

```sh
go run ./cmd/brockchain transfer \
  --wallet alice.wallet.json \
  --chain <64-hex-character-chain-id> \
  --to <64-hex-character-bob-address> \
  --amount 3 --nonce 0 --memo "Thanks for lunch!"
```

The command returns JSON with the canonical signed transaction hex and transaction ID. It does not submit, mine, or confirm it. The nonce must match the sender's next account nonce when executed; an offline wallet cannot check current balances or nonces. A deployed genesis file and network are not created yet.

## Packages and specification

Executable entry points live in `cmd/`; implementation packages live in `internal/`, which prevents external projects from importing them.

- `internal/protocol`: canonical genesis, transfer, block, and target-update formats, hashes, and signature verification.
- `internal/chain`: candidate assembly, nonce search, block validation, reward issuance, and replay.
- `internal/wallet`: random Ed25519 keys, wallet save/load, and signed transfer construction.
- `internal/state`: balances and next nonces, genesis initialization, independent state copies, and validation-before-mutation transfer execution. State is in memory and not concurrency-safe; a future node must serialize access.
- `cmd/brockchain`: wallet commands, offline transfer signing, and the account-state demo.
- [protocol.md](protocol.md): byte formats, cryptography, and genesis identity.
- [NODE_DESIGN.md](NODE_DESIGN.md): execution and consensus design.
- [ROADMAP.md](ROADMAP.md): completed and upcoming stages.
- [testdata/README.md](testdata/README.md): language-independent fixtures and Python reference verification.

Go tests consume the committed genesis/transfer fixtures independently of the Python reference tool. Chain tests consume the block and authority-update fixtures, and exercise atomic validation, next-block target activation, and replay. Rejection tests cover wrong signatures/chain IDs/nonces, replay, overspending, self-transfers, zero amounts, malformed memos, and overflow; failed execution must leave state unchanged.
