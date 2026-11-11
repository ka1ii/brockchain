# Blockchain Protocol Roadmap

## Goal

Build a small blockchain from scratch to demonstrate how independent nodes validate transactions, maintain shared state, mine blocks, and converge after temporarily disagreeing.

The central demo: friends run nodes, mine competing blocks containing conflicting transfers, reconnect, and watch their nodes converge on one valid history.

## Starting design

- **Language:** Go.
- **Network:** A learning network with play money and a handful of participants. Anyone can join and mine.
- **State model:** Accounts with integer balances and a next transaction nonce.
- **Signatures:** Ed25519, using standard cryptographic libraries.
- **Consensus:** Proof of work with signed target updates controlled by a genesis authority key.
- **Fork choice:** Follow the valid branch with the most accumulated work. Keep the current branch on a tie.
- **Transport:** HTTP with configured peers and a bootstrap peer.
- **Persistence:** Store blocks and their parent relationships, including competing branches. Rebuild state by replaying blocks from genesis initially.
- **Interface:** CLI commands for wallets, nodes, transfers, mining, and inspection.

We will implement the protocol rules ourselves and use standard cryptographic primitives. This is an educational implementation; production deployment is outside the initial scope.

## Major pieces

| Piece | What we build |
| --- | --- |
| Protocol format and genesis | Canonical transaction and block-header encoding, a chain ID, shared starting balances, authority-configurable difficulty, and genesis identity. Headers commit to the parent hash, ordered transactions, and mining nonce. |
| Wallets and transactions | Key generation, address derivation, and signed transfers containing sender, recipient, amount, account nonce, chain ID, and a signed UTF-8 memo of up to 256 bytes. |
| State and validation | Deterministic balance and nonce updates; reject invalid signatures, wrong nonces, insufficient funds, and arithmetic overflow. |
| Block production and consensus | Candidate block construction, proof-of-work mining, independent block validation, block reward rules, and fork choice. |
| Peer networking and sync | Exchange transactions and blocks, fetch missing ancestors, and independently validate history received from peers. |
| Storage and reorganizations | Persist branches, switch to the winning branch's state, and reconsider displaced transactions for the mempool. |
| Friend-facing interface | Create wallets, start nodes, send transfers, mine blocks, and inspect balances, peers, chain height, and tip hash. |

## Protocol principles

1. **Validity and fork choice are separate.** Two blocks can both be valid against the same parent. Fork choice selects which valid branch to build on. A longer invalid branch is never accepted.
2. **Encoding is part of the protocol.** All nodes must sign and hash identical bytes. Define field order, integer widths, byte order, length encoding, and signing domains explicitly.
3. **State transitions are deterministic.** Validate and apply transactions in block order against the parent block's state. Every node must produce the same result.
4. **Replay protection has two layers.** Account nonces prevent repeated execution on the same chain; including the chain ID in signatures separates networks with different chain IDs.
5. **Temporary disagreement is expected.** Equal-work valid branches can coexist until one accumulates more work. Proof of work provides increasing confidence rather than immediate finality.
6. **Reorganizations affect the mempool.** Reconsider displaced transactions against the new state, excluding transactions already confirmed or no longer valid.
7. **Peers supply data, not trust.** Each node validates received history, including during initial synchronization.

## Build stages

### 1. Write the protocol specification

- [x] Define transaction fields and exact signing bytes.
- [x] Define address derivation and canonical encoding.
- [x] Define block headers and commitment to ordered transactions.
- [x] Define genesis, chain ID, initial balances, and initial mining target and authority public key.
- [x] Define reward issuance, amount limits, block limits, and overflow handling.
- [x] Define transaction validation, block validation, and fork choice.
- [x] Create shared encoding, hashing, and signature test vectors.

**Status:** Complete as an initial specification. Formats are in [protocol.md](protocol.md), execution and consensus rules in [NODE_DESIGN.md](NODE_DESIGN.md), and verified reference vectors in [testdata/protocol-v1.json](testdata/protocol-v1.json). Actual network keys and allocations are launch inputs, not the public fixture keys.

**Done when:** The rules are precise enough for two independent implementations to agree on hashes and validity.

### 2. Build wallets and account state

- [x] Generate and save Ed25519 key pairs.
- [x] Derive addresses and construct signed transfers.
- [x] Track balances and next account nonces.
- [x] Validate and apply transfers deterministically.
- [x] Test invalid signatures, replay, wrong chain IDs, overspending, and numeric boundaries.

**Status:** Complete. Go packages implement wallet creation/save/load, canonical genesis and transfer encoding, signed memos, account balances/nonces, and atomic transfer validation. Go tests pass against the committed genesis/transfer fixtures and exercise replay, malformed signatures/memos, wrong chains/nonces, overspending, and numeric boundaries. Run `go run ./cmd/brockchain demo` for the offline state demo; no mining or networking is involved yet.

**Done when:** Alice can sign a transfer to Bob and the state updates correctly, while invalid transfers are rejected.

### 3. Build blocks and mining

- [x] Construct candidate blocks from pending transactions.
- [x] Implement header hashing and proof of work using the parent state target.
- [x] Validate transaction commitments, parent references, proof of work, and reward rules.
- [x] Validate a complete block before committing its state changes.
- [x] Replay a chain from genesis to recover account state.

**Status:** Complete. Run `go run ./cmd/brockchain mine-demo` to assemble, mine, validate, and replay a block with a signed transfer. Tests verify canonical block and target-update fixtures, atomic rejection, and next-block target activation.

**Done when:** One offline node mines a signed transfer and reconstructs the same balances by replaying its chain.

### 4. Add branches and persistence

- [ ] Persist blocks indexed by hash and parent.
- [ ] Preserve competing valid branches.
- [ ] Implement greatest-chainwork valid branch selection and keep-current-tip tie behavior.
- [ ] Rebuild state when switching branches.
- [ ] Reconsider displaced transactions for the pending pool.
- [ ] Restore and validate persisted history on restart.

**Done when:** A node switches to a valid branch with more accumulated work, computes the correct balances, and recovers consistently after restart.

### 5. Connect nodes and synchronize

- [ ] Add configured peers and HTTP endpoints.
- [ ] Propagate signed transactions and mined blocks with duplicate suppression.
- [ ] Fetch missing parent blocks and synchronize from genesis.
- [ ] Independently validate all received blocks.
- [ ] Set basic request and message-size limits.
- [ ] Document how friends connect through a shared VPN or reachable host.

**Done when:** A transaction submitted to node A is mined by node B, both converge, and a fresh node C catches up from a peer.

### 6. Deliver the friend-facing fork demo

- [ ] Provide CLI commands to create wallets, start nodes, send transfers, and mine blocks.
- [ ] Show balances, nonces, peers, height, and full tip hash.
- [ ] Provide a repeatable local three-node setup.
- [ ] Document and exercise the deliberate-fork scenario below.

**Done when:** The full demo can be repeated from a clean setup and each node's behavior can be explained using the protocol rules.

## End-to-end milestones

### Milestone A: One node, offline

Create two wallets, fund one at genesis, sign a transfer, mine it, and replay the chain to recover both balances.

### Milestone B: Two nodes, online

Submit a transfer to node A, mine it on node B, and verify that both independently reach the same tip and balances. Start a fresh node and let it synchronize from a peer.

### Milestone C: Three nodes, deliberate fork

1. Start all nodes on the same chain tip.
2. Temporarily disconnect two miners from each other.
3. Create two different signed transfers from the same sender using the same account nonce.
4. Mine one transfer on each branch. Each transfer is valid against the common parent state, but both cannot execute in the same history.
5. Reconnect the network and observe the competing tips at equal height.
6. Extend one branch by another valid block.
7. Verify that all nodes converge on that branch, only the winning transfer affects balances, and the conflicting displaced transfer is excluded from the mempool.

This is the demonstration the project is built around.

## Deferred features

- Smart contracts and a virtual machine.
- Automatic difficulty adjustment (signed authority updates are in scope).
- Transaction fees and a fee market.
- Automatic peer discovery and NAT traversal.
- Optimized state storage, snapshots, and pruning.
- Explicit finality and authority checkpoints (v1 uses probabilistic finality only).
- Alternative consensus mechanisms and production-scale hardening.
- A graphical explorer.

## Authority-controlled difficulty

The genesis authority key can sign a target update bound to a parent block. A miner includes it in a block mined under the existing target; children use the new target. Nodes verify signatures and replay updates as branch state. Fork choice uses accumulated work, since block count is insufficient when difficulty varies. Difficulty updates are branch-local: a branch that forks before an update can retain its previous target, but easier blocks contribute less work. Add tests for update authorization, activation, replay protection, reorganizations, and unequal-difficulty forks. For the original fork demo, keep the target unchanged throughout.

## Next step

Begin stage 4: store competing branches, select the valid tip with the most accumulated work, and persist and restore chain history.
