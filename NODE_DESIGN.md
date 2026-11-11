# Brockchain node behavior — design draft

These rules are preserved from the original broad specification. Canonical formats and genesis are defined in [protocol.md](protocol.md). This document covers later roadmap stages: validation, mining, consensus, networking, and storage.

## Transfer execution

An absent account has balance 0 and next nonce 0. To apply a transfer:

1. Require version 1 and the local chain ID; require a memo of at most 256 bytes containing valid UTF-8, with an exact encoded length.
2. Verify the signature under the sender public key.
3. Require positive amount and different sender and recipient addresses.
4. Require the nonce to equal the sender's next nonce, and that nonce to be less than `2^64 - 1`.
5. Require sufficient sender funds and a recipient balance addition that fits `u64`.
6. Subtract the amount, add it to the recipient, and increment the sender's next nonce.

All checks precede mutation. Nonces begin at zero. Transfers may spend funds received earlier in the same block. Account nonces and mining nonces are separate concepts.

## Mining

A miner varies `mining_nonce` until the block hash is **less than or equal to** the active target in the parent's state. On nonce exhaustion, change the candidate transaction list or miner address to obtain another search space.

## Authority-update execution

An included update must have version 1, the local chain ID, the containing block's parent hash, a valid authority signature, and sequence exactly parent sequence plus one without overflow. Require `1 <= new_target <= 2^256 - 1`. At most one update is permitted per block. A larger target makes mining easier; a smaller target makes it harder.

The containing block is mined under the **old** target from its parent state. After validation, its state adopts the new target and sequence, so its children use the new target. Without an update, both values carry forward. Binding updates to a parent prevents replay on unrelated branches; extending that parent may include the same update in competing sibling blocks.

The operator signs an update for the current tip and submits it to miners. Inclusion requires a valid mined block; signing alone does not activate it. If the tip changes before inclusion, the operator may need to sign a replacement for the new parent. Reorganizations restore the target and sequence of the winning branch. An update has no special finality.

The authority is trusted to choose usable difficulty: it can make mining effectively trivial or stall it. There is no out-of-band override; recovering from an impractically hard active target requires mining under that target or coordinating a new protocol/network. Authority-node downtime does not stop normal mining at the last active target.

## 6. Block validation and state transition

For each non-genesis block:

1. Check canonical encoding, version, chain ID, and transfer count limit.
2. Resolve a known valid parent; unknown-parent blocks are pending data, not accepted history.
3. Require parent height less than `2^64 - 1` and child height exactly parent height plus one.
4. Recompute both commitments, validate any authority update, and check proof of work against the parent's active target.
5. Reject duplicate transaction IDs within the block.
6. Starting from the parent's state, validate and apply every transfer in order using the transfer rules above.
7. Credit exactly 50 units to the header's miner address after all transfers, rejecting balance or total-supply overflow.

Apply the target update to the resulting state after these checks. Accept the block and resulting state atomically only if every step succeeds. An invalid transfer invalidates the entire block. The reward is implicit, has no signature or account nonce, and cannot be spent within its own block. Miner rewards do not increment account nonces. Empty blocks are valid and receive rewards. Total supply is genesis supply plus 50 times height and must fit `u64`.

## 7. Fork choice and reorganizations

Store all validated blocks by hash, including side branches. Block work is `floor(2^256 / (target_used_for_this_block + 1))`; genesis work is zero. Chainwork is the sum along a branch, computed with arbitrary-precision integers. Choose the valid tip with greatest chainwork, not greatest height. Keep the current tip on equal chainwork. If multiple strictly better tips share maximum chainwork during one selection, choose the lexicographically smallest hash among them.

Difficulty updates are branch-local state, not global commands. A branch that forks before an update can retain the previous target. Its blocks remain valid if they satisfy that branch's target, but each easier block contributes less work. A difficulty increase does not invalidate earlier targets on competing branches, guarantee that its branch wins, or finalize the update. Only accumulated work selects the active history.

A reorganization finds the common ancestor, switches the active branch, and replaces active state with a replay of the winning branch from genesis. Persist the active tip consistently with its state. An invalid branch never participates, regardless of height or claimed chainwork.

After switching, remove transactions confirmed on the new branch. Reconsider other pending transactions and transfers displaced from the old branch against the new state. Replays, conflicts, and insufficient funds may make formerly valid transactions unusable. Balances and confirmations can change after a reorganization; there is no irreversible finality rule.

## 8. Mempool policy (local)

The mempool is not consensus state. Initially cap it at 10,000 transfers, reject duplicate IDs, and retain at most one candidate per `(sender, nonce)`, keeping the first seen. Only admit transfers executable against the current confirmed state; future nonces can be resubmitted later.

For candidate blocks and mempool reconstruction, sort candidates by transaction ID, repeatedly apply executable candidates to a scratch state, and stop when no progress is possible or the block limit is reached. Discard leftovers during reconstruction; users may resubmit them. This deliberately simple policy may discard dependent or conflicting transfers. A block with valid ordered transfers must still be accepted even if local mempool policy would not admit those transfers.

## 9. Peer transport and synchronization

Initial HTTP API uses hex-encoded canonical bytes in JSON envelopes. JSON field order never affects consensus hashes.

| Endpoint | Contract |
| --- | --- |
| `GET /v1/status` | Version, chain ID, active height, tip hash, chainwork (decimal string), active target and update sequence |
| `POST /v1/transactions` | `{ "transaction": "<hex>" }`; validate and relay a newly admitted transfer |
| `POST /v1/target-updates` | `{ "update": "<hex>" }`; verify against current tip and relay to miners |
| `POST /v1/blocks` | `{ "block": "<hex>" }`; validate known-parent blocks and relay newly accepted blocks |
| `GET /v1/blocks/{hash}` | `{ "block": "<hex>" }` for a stored block; 404 if absent |

Use 200 for successful reads, 202 for admitted submissions, 400 for malformed or invalid data, 409 for an unknown parent, and 404 for missing resources. Duplicate known submissions return 200 without relaying again. Target updates for a different parent return 409; each node retains at most one pending valid update for its current tip, keeping first seen, and clears it when the tip changes. Miners may include it in a candidate block. No authority private key is exposed through HTTP. Peer URLs are configured locally; bootstrap peers are ordinary untrusted peers.

Check chain ID before syncing. Query peer status, fetch its tip and missing ancestors by hash until reaching known history, then validate forward. Never trust claimed height, chainwork, target, or balances; derive these by validation and replay. Unknown-parent announcements trigger ancestor fetching; the sender can resubmit afterward. Poll peer status periodically to recover from missed announcements. Genesis configuration is shared separately, not downloaded as a normal block.

Local limits: 1 MiB HTTP bodies (the maximum block is 436,353 binary bytes, or 872,706 hex characters before its JSON envelope), request timeouts, bounded concurrent fetches, and at most 1,000 downloaded ancestors per sync batch. Continue subsequent batches for longer chains. Never discard validated competing branches merely because an equal-work tip was retained. These are resource policies, not validity rules.

## 10. Persistence and inspection

Persist canonical genesis configuration and accepted block bytes, including side branches, plus the chosen active tip. On restart, verify genesis identity and replay stored history through the same validation code. Reject corrupt records rather than silently trusting cached state. Wallet storage is separate from chain storage.

CLI inspection must expose chain ID, active height, tip hash, account balance and next nonce, peers, active target, update sequence, chainwork, and known competing tips. Show transaction confirmation relative to the active branch.

## 11. Required verification

- Fixed vectors for configuration encoding, chain ID, transfer signing bytes, transaction ID, transaction commitment, header encoding, and block hash.
- Test empty, multibyte, maximum-length, oversized, malformed, and signature-tampered memos.
- Boundary tests for nonces, balances, supply overflow, malformed encodings, and proof-of-work target comparisons.
- Reject altered signatures, wrong-chain transfers, replay, overspending, invalid commitments, and invalid rewards/state transitions.
- Verify authority signatures, parent binding, update sequencing, commitment tampering, next-block activation, target bounds, and target restoration on reorganization.
- Verify that a shorter branch with more accumulated work wins over a longer easier branch.
- Verify that a competing branch without a difficulty update uses its own inherited target and that a reorganization restores the winning branch's target.
- Replay and restart reproduce the same tip and account state.
- Two online nodes independently converge; a fresh node syncs and validates history.
- Three-node partition demo: conflicting same-nonce transfers appear on separate valid branches; extending one branch causes convergence and excludes the losing transfer from active balances and the reconstructed mempool.
