# Protocol v1 test vectors

`protocol-v1.json` fixes canonical bytes, signing messages, signatures, IDs, and hashes for [protocol.md](../protocol.md). These are reference fixtures for future Go tests, not a deployed genesis configuration.

## Run

From the repository root:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r tools/requirements.txt
.venv/bin/python tools/protocol_vectors.py
```

On Windows, use `.venv\Scripts\python.exe` in place of `.venv/bin/python`.

The default command checks the saved fixtures without replacing them. It also decodes the records, verifies signatures, rejects malformed cases, checks mined proofs of work, exercises the maximum encoded block size, and checks Ed25519 against the RFC 8032 section 7.1 empty-message vector. Generation and parsing are separate paths, but use one reference tool; later Go tests should consume the committed expected results directly to provide an independent implementation check.

Only deliberately regenerate after reviewing a specification change:

```sh
.venv/bin/python tools/protocol_vectors.py --write
```

## Contents and conventions

- Three genesis configurations: funded, empty, and maximum starting supply.
- Six transfers: empty, ASCII, multibyte UTF-8, 256 ASCII bytes, 256 multibyte UTF-8 bytes, and maximum u64 amount/nonce fields.
- Three signed authority updates: harder target, maximum target, and minimum target.
- Two mined height-one blocks extending the funded genesis: an empty block, and an alternative block containing a transfer and signed difficulty update. The updated target applies to children, not the carrying block.
- Rejection cases cover malformed lengths, versions, chain IDs, UTF-8, signatures, genesis allocations, commitments, presence flags, counts, and proof of work.
- Explicit field sizes and limits.

Hex strings represent raw bytes. Large integer fields are decimal strings so JSON consumers do not lose precision. Keys come from deterministic 32-byte Ed25519 seeds. **Seeds are public fixture data: never use these wallets or authority keys for the deployed network.**

Format validity is distinct from execution validity. The maximum-u64 transfer encodes correctly but is not executable against the funded genesis; its nonce cannot increment. The maximum-supply genesis cannot accept further block rewards. The generated maximum-size block repeats one transfer to exercise parsing and size calculations; execution must reject those duplicates. Balance, nonce, reorganization, and supply-transition tests remain work for the implementation stages.

These vectors describe the initial v1 draft. No deployed network compatibility is being asserted.
