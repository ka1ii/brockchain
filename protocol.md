# Brockchain protocol format and genesis — v1

Status: initial format specification for the first roadmap stage. This document defines canonical bytes, signatures, hashes, genesis, and network identity. Execution, fork choice, mempool, networking, and storage rules live in [NODE_DESIGN.md](NODE_DESIGN.md).

## 1. Network parameters

| Parameter | Value |
| --- | --- |
| Protocol version | 1 |
| Chain ID | 32 bytes, derived from genesis |
| Block reward | 50 integer base units per non-genesis block |
| Initial mining target | Hex `0000ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff` |
| Maximum transfers per block | 1,000 |
| Maximum memo length | 256 UTF-8 bytes |
| Maximum genesis accounts | 1,000 |
| Authority | One Ed25519 public key committed in genesis |

The authority can sign mining-target updates without changing the chain ID. Other parameters are immutable for this network. There is no automatic difficulty adjustment. Concrete starting balances, authority key, and network nonce will be chosen when initializing the demo.

## 2. Canonical encoding and cryptography

- `u8`, `u16`, `u32`, and `u64` are unsigned integers of 1, 2, 4, and 8 bytes. Multibyte integers use big-endian encoding.
- `bytes[N]` is exactly N raw bytes. `||` means concatenation in the stated order.
- No implicit lengths, padding, whitespace, or JSON appear in canonical bytes. Variable lists have explicit counts.
- Decoders reject truncated input, trailing bytes, invalid lengths, unsupported versions, and invalid presence flags.
- `H(x)` is SHA-256, returning 32 bytes. Domain strings are exact UTF-8 bytes, including the final colon and excluding a null terminator.
- Addresses are raw 32-byte Ed25519 public keys. Signatures are ordinary 64-byte Ed25519 signatures, using Go's standard `crypto/ed25519` implementation.
- Addresses and hashes display as lowercase hex in byte order; display text is not hashed. Targets are 32-byte unsigned big-endian integers.

Private keys are never part of genesis, transactions, blocks, or peer messages.

## 3. Genesis configuration

All nodes receive the same immutable configuration before joining. Its canonical fields, in order, are:

| Field | Encoding | Meaning |
| --- | --- | --- |
| version | u16 | 1 |
| network_nonce | bytes[32] | Shared random network identifier |
| authority_public_key | bytes[32] | Public key authorized to sign target updates |
| target | bytes[32] | Initial target from section 1 |
| reward | u64 | 50 |
| max_transfers | u32 | 1,000 |
| max_genesis_accounts | u32 | 1,000 |
| allocation_count | u32 | Number of following allocations, from 0 to 1,000 |
| allocations | allocation_count × 40 bytes | Ordered starting balances |

Each allocation is `address:bytes[32] || balance:u64`. Addresses must be unique and sorted lexicographically by unsigned raw bytes. Balances must be positive and their sum must fit u64. Zero allocations are allowed; mining can supply the initial coins.

The configuration has a 118-byte fixed prefix, followed by 40 bytes per allocation. Parameter fields must match section 1. The network nonce distinguishes networks with otherwise identical settings.

### Chain ID and genesis identity

```text
chain_id = H("brockchain/genesis/v1:" || configuration_bytes)
genesis_hash = chain_id
```

Genesis is a special checkpoint at height 0, not an ordinary block. It has no parent, transfers, mining nonce, proof of work, or reward. Its initial account balances are the allocations, every account's next nonce is zero, the active target is the configured target, and the authority-update sequence is zero.

Changing any genesis byte produces a different network identity. A shared genesis file may use a human-readable representation, but its identity is always computed from the canonical bytes above, never from JSON formatting. The exact file wrapper is deferred to implementation.

## 4. Transfer format

Unsigned transfer fields, in order:

| Field | Encoding | Meaning |
| --- | --- | --- |
| version | u16 | 1 |
| chain_id | bytes[32] | Network this transfer belongs to |
| sender | bytes[32] | Sender public key/address |
| recipient | bytes[32] | Recipient public key/address |
| amount | u64 | Integer base units |
| nonce | u64 | Sender's account transaction sequence |
| memo_length | u16 | Number of following memo bytes, from 0 to 256 |
| memo | bytes[memo_length] | UTF-8 message; empty when memo_length is 0 |

Unsigned transfers are `116 + memo_length` bytes (116–372 bytes). Sign with the sender's private key:

```text
signing_message = "brockchain/transfer/v1:" || unsigned_transfer_bytes
full_transfer_bytes = unsigned_transfer_bytes || signature:bytes[64]
transaction_id = H("brockchain/txid/v1:" || full_transfer_bytes)
```

Full transfers are `180 + memo_length` bytes (180–436 bytes). The memo length and exact memo bytes are included in the signing message and transaction ID. Decoders reject lengths above 256, truncated memos, and invalid UTF-8. No Unicode normalization is applied: different byte sequences remain different messages. The limit counts bytes, not characters.

Memos are public transaction data retained with their blocks with no effect on balance calculations or execution. They are not encrypted. A memo accompanies a normal positive-amount transfer; this does not introduce message-only transactions. Clients should render memos as plain text.

Account nonces are distinct from mining nonces. Balance checks and nonce execution are specified in the node design.

## 5. Authority target-update format

Unsigned update fields, in order:

| Field | Encoding | Meaning |
| --- | --- | --- |
| version | u16 | 1 |
| chain_id | bytes[32] | Network being configured |
| parent_hash | bytes[32] | Parent of the block that may carry this update |
| sequence | u64 | Authority-update sequence |
| new_target | bytes[32] | New target, from 1 through 2^256 − 1 |

The unsigned update is 106 bytes. Sign with the genesis authority private key:

```text
signing_message = "brockchain/target/v1:" || unsigned_update_bytes
full_update_bytes = unsigned_update_bytes || signature:bytes[64]
```

A full update is 170 bytes. A block carries at most one. The carrying block uses the existing target; the update takes effect for its children. Detailed sequencing and branch behavior are specified in the node design.

## 6. Block header

Canonical non-genesis header fields, in order:

| Field | Encoding | Meaning |
| --- | --- | --- |
| version | u16 | 1 |
| chain_id | bytes[32] | Network identity |
| parent_hash | bytes[32] | Previous block hash, or genesis hash for height 1 |
| height | u64 | Block height |
| transactions_hash | bytes[32] | Commitment to ordered transfers |
| target_update_hash | bytes[32] | Commitment to update presence and contents |
| miner_address | bytes[32] | Reward recipient |
| mining_nonce | u64 | Value varied during mining |

Headers are 178 bytes. There is no timestamp or explicit target field; the applicable target is derived from the parent state.

```text
block_hash = H("brockchain/block/v1:" || header_bytes)
```

The parent hash links history. The commitments bind the body to the header, so changing body contents changes the header hash.

## 7. Block body and commitments

Canonical block bytes:

```text
header_bytes || update_present:u8 || optional_update || count:u32 || transfers
```

- `update_present` must be 0 or 1.
- At 0, `optional_update` is empty; at 1, it is exactly one 170-byte full update.
- `count` ranges from 0 to 1,000.
- `transfers` contains exactly `count` full variable-length transfers in execution order. Parse each transfer using its fixed fields, memo_length, memo bytes, and 64-byte signature; no additional per-transfer length prefix is used.

Commitments:

```text
transactions_hash = H("brockchain/txlist/v1:" || count:u32 || transfers)
target_update_hash = H("brockchain/target-commit/v1:" || update_present:u8 || optional_update)
```

An empty transfer list and an absent update still have computed commitments, not all-zero placeholders. The transfer commitment covers ordering and signatures; a Merkle tree is deferred.

A block with no transfers or update is 183 bytes. The maximum block size is 436,353 bytes: 178 header + 1 flag + 170 update + 4 count + 1,000 × 436-byte transfers.

## 8. Format verification checklist

Before moving to execution logic, create fixed vectors for:

- Genesis encoding and chain ID.
- Transfer encoding, signing message, signature, and transaction ID, including empty memos, multibyte UTF-8, and the 256-byte boundary.
- Reject oversized or invalid UTF-8 memos, inconsistent lengths, and memo tampering after signing.
- Authority-update encoding and signing message.
- Empty and populated body commitments.
- Header encoding and block hash.
- Exact lengths, boundary integers, malformed encodings, unsupported versions, and trailing-byte rejection.

These vectors will establish that independent nodes hash and sign identical bytes.

## Reference fixtures

The stage-one reference vectors are committed in [testdata/protocol-v1.json](testdata/protocol-v1.json), with reproduction and verification instructions in [testdata/README.md](testdata/README.md). They use public test keys, not launch credentials. V1 provides probabilistic finality only; no authority checkpoint mechanism is included.
