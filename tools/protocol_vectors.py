"""Reference fixtures for protocol.md. Public deterministic keys: TEST USE ONLY."""
import argparse
import hashlib
import json
from pathlib import Path
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey, Ed25519PublicKey

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'testdata/protocol-v1.json'
TARGET = bytes.fromhex('0000' + 'ff' * 30)
U64 = (1 << 64) - 1

def h(domain, value):
    return hashlib.sha256(('brockchain/' + domain + '/v1:').encode() + value).digest()

def u(value, width):
    return value.to_bytes(width, 'big')

def key(seed):
    private = Ed25519PrivateKey.from_private_bytes(seed)
    public = private.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
    return private, public

AUTH, AUTH_PUB = key(bytes(range(32)))
ALICE, ALICE_PUB = key(bytes(range(32, 64)))
BOB, BOB_PUB = key(bytes(range(64, 96)))

class Reader:
    def __init__(self, data):
        self.data, self.pos = data, 0

    def take(self, count):
        if self.pos + count > len(self.data):
            raise ValueError('truncated encoding')
        result = self.data[self.pos:self.pos + count]
        self.pos += count
        return result

    def number(self, width):
        return int.from_bytes(self.take(width), 'big')

    def end(self):
        if self.pos != len(self.data):
            raise ValueError('trailing bytes')

def version(r):
    if r.number(2) != 1:
        raise ValueError('unsupported version')

def decode_genesis(data):
    r = Reader(data)
    version(r)
    r.take(32)  # network nonce
    r.take(32)  # authority key
    if r.take(32) != TARGET or r.number(8) != 50 or r.number(4) != 1000 or r.number(4) != 1000:
        raise ValueError('incorrect genesis parameters')
    count = r.number(4)
    if count > 1000:
        raise ValueError('too many allocations')
    previous, total = None, 0
    for _ in range(count):
        address, balance = r.take(32), r.number(8)
        if previous is not None and address <= previous:
            raise ValueError('unsorted or duplicate allocation')
        if balance == 0:
            raise ValueError('zero allocation')
        total += balance
        if total > U64:
            raise ValueError('supply overflow')
        previous = address
    r.end()

def decode_transfer(r, chain):
    start = r.pos
    version(r)
    if r.take(32) != chain:
        raise ValueError('wrong chain')
    sender = r.take(32)
    r.take(32)  # recipient
    r.number(8)  # amount; state validity is separate from format
    r.number(8)  # nonce
    length = r.number(2)
    if length > 256:
        raise ValueError('oversized memo')
    r.take(length).decode('utf-8', errors='strict')
    unsigned = r.data[start:r.pos]
    signature = r.take(64)
    Ed25519PublicKey.from_public_bytes(sender).verify(signature, b'brockchain/transfer/v1:' + unsigned)
    return r.data[start:r.pos]

def decode_update(r, chain, parent=None):
    start = r.pos
    version(r)
    if r.take(32) != chain:
        raise ValueError('wrong chain')
    actual_parent = r.take(32)
    if parent is not None and parent != actual_parent:
        raise ValueError('wrong parent')
    r.number(8)
    if r.number(32) == 0:
        raise ValueError('zero target')
    unsigned = r.data[start:r.pos]
    AUTH.public_key().verify(r.take(64), b'brockchain/target/v1:' + unsigned)
    return r.data[start:r.pos]

def decode_block(data, chain, target):
    r = Reader(data)
    version(r)
    if r.take(32) != chain:
        raise ValueError('wrong chain')
    parent = r.take(32)
    r.number(8)
    tx_hash, update_hash = r.take(32), r.take(32)
    r.take(32)
    r.number(8)
    header = data[:r.pos]
    flag = r.number(1)
    if flag not in (0, 1):
        raise ValueError('invalid update flag')
    update = decode_update(r, chain, parent) if flag else b''
    if h('target-commit', u(flag, 1) + update) != update_hash:
        raise ValueError('update commitment mismatch')
    start = r.pos
    count = r.number(4)
    if count > 1000:
        raise ValueError('too many transfers')
    for _ in range(count):
        decode_transfer(r, chain)
    if h('txlist', data[start:r.pos]) != tx_hash:
        raise ValueError('transaction commitment mismatch')
    r.end()
    if int.from_bytes(h('block', header), 'big') > int.from_bytes(target, 'big'):
        raise ValueError('invalid proof of work')

def genesis(allocations):
    return (u(1, 2) + bytes(range(96, 128)) + AUTH_PUB + TARGET + u(50, 8)
            + u(1000, 4) + u(1000, 4) + u(len(allocations), 4)
            + b''.join(address + u(balance, 8) for address, balance in allocations))

def transfer(chain, memo, amount=3, nonce=0):
    raw = u(1, 2) + chain + ALICE_PUB + BOB_PUB + u(amount, 8) + u(nonce, 8) + u(len(memo), 2) + memo
    message = b'brockchain/transfer/v1:' + raw
    signature = ALICE.sign(message)
    full = raw + signature
    return dict(amount=str(amount), nonce=str(nonce), memo_hex=memo.hex(), unsigned_hex=raw.hex(),
                signing_message_hex=message.hex(), signature_hex=signature.hex(), full_hex=full.hex(),
                size_bytes=len(full), transaction_id=h('txid', full).hex())

def update(chain, parent, target, sequence=1):
    raw = u(1, 2) + chain + parent + u(sequence, 8) + target
    message = b'brockchain/target/v1:' + raw
    signature = AUTH.sign(message)
    return dict(unsigned_hex=raw.hex(), signing_message_hex=message.hex(), signature_hex=signature.hex(),
                full_hex=(raw + signature).hex(), size_bytes=len(raw) + 64,
                sequence=str(sequence), new_target_hex=target.hex())

def block(chain, transfers, upd=b''):
    txlist = u(len(transfers), 4) + b''.join(transfers)
    optional = u(bool(upd), 1) + upd
    txhash, uphash = h('txlist', txlist), h('target-commit', optional)
    prefix = u(1, 2) + chain + chain + u(1, 8) + txhash + uphash + BOB_PUB
    nonce = 0
    while True:
        header = prefix + u(nonce, 8)
        digest = h('block', header)
        if digest <= TARGET:
            break
        nonce += 1
    full = header + optional + txlist
    return dict(height=1, mining_nonce=str(nonce), target_used_hex=TARGET.hex(),
                block_work=str((1 << 256) // (int.from_bytes(TARGET, 'big') + 1)),
                header_hex=header.hex(), header_size_bytes=len(header), full_hex=full.hex(),
                size_bytes=len(full), block_hash=digest.hex(), transactions_hash=txhash.hex(),
                target_update_hash=uphash.hex())

def generate():
    allocations = sorted([(ALICE_PUB, 100), (BOB_PUB, 25)])
    g = genesis(allocations)
    chain = h('genesis', g)
    transfers = {name: transfer(chain, memo) for name, memo in [
        ('empty', b''), ('ascii', b'Thanks for lunch!'), ('unicode', 'Hello 🌍'.encode()),
        ('max_ascii', b'x' * 256), ('max_multibyte', ('🌍' * 64).encode())]}
    transfers['u64_max_fields'] = transfer(chain, b'', U64, U64)
    upd = update(chain, chain, bytes.fromhex('000000' + 'ff' * 29))
    records = {}
    for name, alloc in [('funded', allocations), ('empty', []), ('max_supply', [(ALICE_PUB, U64)])]:
        raw = genesis(alloc)
        records[name] = dict(configuration_hex=raw.hex(), size_bytes=len(raw), chain_id=h('genesis', raw).hex(),
                             allocations=[dict(address=a.hex(), balance=str(b)) for a, b in alloc])
    invalid = []
    def bad(name, kind, data):
        invalid.append(dict(name=name, kind=kind, full_hex=data.hex()))
    tx = bytes.fromhex(transfers['ascii']['full_hex'])
    bad('transfer_truncated_signature', 'transfer', tx[:-1])
    bad('transfer_trailing_byte', 'transfer', tx + b'\0')
    bad('transfer_version', 'transfer', b'\0\2' + tx[2:])
    bad('transfer_wrong_chain', 'transfer', tx[:2] + bytes(32) + tx[34:])
    bad('transfer_memo_tampered', 'transfer', tx[:116] + b'X' + tx[117:])
    bad('transfer_invalid_utf8_signed', 'transfer', bytes.fromhex(transfer(chain, b'\xff')['full_hex']))
    bad('transfer_oversized_signed', 'transfer', bytes.fromhex(transfer(chain, b'x' * 257)['full_hex']))
    bad('transfer_memo_length_mismatch', 'transfer', tx[:114] + u(256, 2) + tx[116:])
    bad('genesis_duplicate_addresses', 'genesis', genesis([(ALICE_PUB, 1), (ALICE_PUB, 2)]))
    bad('genesis_unsorted', 'genesis', genesis(list(reversed(allocations))))
    bad('genesis_zero_balance', 'genesis', genesis([(ALICE_PUB, 0)]))
    bad('genesis_supply_overflow', 'genesis', genesis(sorted([(ALICE_PUB, U64), (BOB_PUB, 1)])))
    bad('genesis_truncated', 'genesis', g[:-1])
    bad('genesis_count_over_limit', 'genesis', g[:114] + u(1001, 4) + g[118:])
    bad('update_zero_target_signed', 'update', bytes.fromhex(update(chain, chain, bytes(32))['full_hex']))
    update_raw = bytes.fromhex(upd['full_hex'])
    bad('update_tampered_signature', 'update', update_raw[:-1] + bytes([update_raw[-1] ^ 1]))
    blocks = dict(empty=block(chain, []), populated=block(chain, [bytes.fromhex(transfers['ascii']['full_hex'])], update_raw))
    raw = bytes.fromhex(blocks['empty']['full_hex'])
    bad('block_invalid_flag', 'block', raw[:178] + b'\2' + raw[179:])
    bad('block_count_over_limit', 'block', raw[:179] + u(1001, 4))
    bad('block_trailing_byte', 'block', raw + b'\0')
    altered = bytearray(raw); altered[74] ^= 1
    bad('block_commitment_mismatch', 'block', bytes(altered))
    bad_nonce = 0
    while h('block', raw[:170] + u(bad_nonce, 8)) <= TARGET:
        bad_nonce += 1
    bad('block_invalid_pow', 'block', raw[:170] + u(bad_nonce, 8) + raw[178:])
    return dict(format='brockchain-protocol-v1-vectors', warning='PUBLIC TEST KEYS ONLY. Never fund these keys on a real network.',
                keys={name: dict(seed_hex=seed.hex(), public_key_hex=key(seed)[1].hex()) for name, seed in
                      [('authority', bytes(range(32))), ('alice', bytes(range(32, 64))), ('bob', bytes(range(64, 96)))]},
                genesis=records, chain_id=chain.hex(), transfers=transfers,
                updates=dict(harder=upd, maximum_target=update(chain, chain, b'\xff' * 32), minimum_target=update(chain, chain, u(1, 32))),
                blocks=blocks, invalid=invalid,
                size_limits=dict(genesis_prefix=118, transfer_min=180, transfer_max=436, header=178, update=170, block_min=183, block_max=436353))

def verify(v):
    # Published RFC 8032 section 7.1 empty-message vector checks the signature primitive independently.
    seed = bytes.fromhex('9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60')
    private, public = key(seed)
    assert public.hex() == 'd75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a'
    assert private.sign(b'').hex() == ('e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555f'
                                     'b8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b')
    chain = bytes.fromhex(v['chain_id'])
    def check(kind, data):
        if kind == 'genesis':
            decode_genesis(data)
        elif kind == 'block':
            decode_block(data, chain, TARGET)
        else:
            r = Reader(data)
            if kind == 'transfer':
                decode_transfer(r, chain)
            else:
                decode_update(r, chain, chain)
            r.end()
    for record in v['genesis'].values():
        check('genesis', bytes.fromhex(record['configuration_hex']))
    for kind, records in [('transfer', v['transfers']), ('update', v['updates']), ('block', v['blocks'])]:
        for record in records.values():
            check(kind, bytes.fromhex(record['full_hex']))
    for record in v['invalid']:
        try:
            check(record['kind'], bytes.fromhex(record['full_hex']))
        except (ValueError, InvalidSignature):
            pass
        else:
            raise AssertionError('accepted invalid vector: ' + record['name'])
    # Largest block exercises variable-length parsing and the declared byte limit.
    max_tx = bytes.fromhex(v['transfers']['max_ascii']['full_hex'])
    maximum = block(chain, [max_tx] * 1000, bytes.fromhex(v['updates']['harder']['full_hex']))
    assert maximum['size_bytes'] == v['size_limits']['block_max']
    decode_block(bytes.fromhex(maximum['full_hex']), chain, TARGET)
    # This synthetic maximum-size block intentionally repeats transfers: format-valid,
    # not state-valid. Duplicate and nonce checks belong to later execution tests.
    assert int(v['blocks']['empty']['block_work']) == 65536

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--write', action='store_true', help='explicitly replace the checked-in fixture')
    args = parser.parse_args()
    expected = generate()
    verify(expected)
    if args.write:
        OUT.write_text(json.dumps(expected, indent=2, ensure_ascii=False) + '\n')
        print('Wrote', OUT)
    else:
        saved = json.loads(OUT.read_text())
        assert saved == expected, 'fixture differs; investigate before deliberately regenerating'
        verify(saved)
        print(f"PASS: 3 genesis, {len(saved['transfers'])} transfers, 3 updates, 2 mined blocks, "
              f"{len(saved['invalid'])} rejection cases, RFC 8032 signature, and maximum block size")
