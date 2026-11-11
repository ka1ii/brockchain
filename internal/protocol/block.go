package protocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
)

const HeaderSize = 178
const TargetUpdateSize = 170
const MaxBlockSize = HeaderSize + 1 + TargetUpdateSize + 4 + MaxTransfers*(180+MaxMemo)

type Header struct {
	Version          uint16
	ChainID          Hash
	ParentHash       Hash
	Height           uint64
	TransactionsHash Hash
	TargetUpdateHash Hash
	Miner            Address
	MiningNonce      uint64
}

func (h Header) Bytes() [HeaderSize]byte {
	var data [HeaderSize]byte
	binary.BigEndian.PutUint16(data[:2], h.Version)
	copy(data[2:34], h.ChainID[:])
	copy(data[34:66], h.ParentHash[:])
	binary.BigEndian.PutUint64(data[66:74], h.Height)
	copy(data[74:106], h.TransactionsHash[:])
	copy(data[106:138], h.TargetUpdateHash[:])
	copy(data[138:170], h.Miner[:])
	binary.BigEndian.PutUint64(data[170:], h.MiningNonce)
	return data
}

func (h Header) Hash() Hash {
	data := h.Bytes()
	return Digest("block", data[:])
}

func MeetsTarget(hash, target Hash) bool {
	return target != (Hash{}) && bytes.Compare(hash[:], target[:]) <= 0
}

type TargetUpdate struct {
	Version    uint16
	ChainID    Hash
	ParentHash Hash
	Sequence   uint64
	NewTarget  Hash
	Signature  [64]byte
}

func (u TargetUpdate) UnsignedBytes() ([]byte, error) {
	if u.Version != Version || u.NewTarget == (Hash{}) {
		return nil, errors.New("invalid target update version or target")
	}
	var b bytes.Buffer
	put(&b, u.Version)
	b.Write(u.ChainID[:])
	b.Write(u.ParentHash[:])
	put(&b, u.Sequence)
	b.Write(u.NewTarget[:])
	return b.Bytes(), nil
}

func (u TargetUpdate) SigningMessage() ([]byte, error) {
	data, err := u.UnsignedBytes()
	if err != nil {
		return nil, err
	}
	return append([]byte("brockchain/target/v1:"), data...), nil
}

func (u TargetUpdate) Verify(authority Address) error {
	msg, err := u.SigningMessage()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(authority[:]), msg, u.Signature[:]) {
		return errors.New("invalid authority signature")
	}
	return nil
}

func (u TargetUpdate) MarshalBinary() ([]byte, error) {
	data, err := u.UnsignedBytes()
	if err != nil {
		return nil, err
	}
	return append(data, u.Signature[:]...), nil
}

func DecodeTargetUpdate(data []byte) (TargetUpdate, error) {
	var u TargetUpdate
	if len(data) != TargetUpdateSize {
		return u, errors.New("invalid target update length")
	}
	u.Version = binary.BigEndian.Uint16(data[:2])
	copy(u.ChainID[:], data[2:34])
	copy(u.ParentHash[:], data[34:66])
	u.Sequence = binary.BigEndian.Uint64(data[66:74])
	copy(u.NewTarget[:], data[74:106])
	copy(u.Signature[:], data[106:])
	_, err := u.UnsignedBytes()
	return u, err
}

type Block struct {
	Header    Header
	Update    *TargetUpdate
	Transfers []Transfer
}

func (b Block) bodyParts() (update, transfers []byte, err error) {
	if len(b.Transfers) > MaxTransfers {
		return nil, nil, errors.New("too many transfers")
	}
	update = []byte{0}
	if b.Update != nil {
		raw, err := b.Update.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
		update = append([]byte{1}, raw...)
	}
	var list bytes.Buffer
	put(&list, uint32(len(b.Transfers)))
	for i, tx := range b.Transfers {
		raw, err := tx.MarshalBinary()
		if err != nil {
			return nil, nil, fmt.Errorf("transfer %d: %w", i, err)
		}
		list.Write(raw)
	}
	return update, list.Bytes(), nil
}

func (b Block) Commitments() (transactions, update Hash, err error) {
	updateBytes, transfers, err := b.bodyParts()
	if err != nil {
		return Hash{}, Hash{}, err
	}
	return Digest("txlist", transfers), Digest("target-commit", updateBytes), nil
}

func (b Block) MarshalBinary() ([]byte, error) {
	if b.Header.Version != Version {
		return nil, errors.New("unsupported block version")
	}
	update, transfers, err := b.bodyParts()
	if err != nil {
		return nil, err
	}
	header := b.Header.Bytes()
	return append(append(header[:], update...), transfers...), nil
}

func DecodeBlock(data []byte) (Block, error) {
	var b Block
	if len(data) < HeaderSize+5 || len(data) > MaxBlockSize {
		return b, errors.New("invalid block length")
	}
	h := &b.Header
	h.Version = binary.BigEndian.Uint16(data[:2])
	if h.Version != Version {
		return b, errors.New("unsupported block version")
	}
	copy(h.ChainID[:], data[2:34])
	copy(h.ParentHash[:], data[34:66])
	h.Height = binary.BigEndian.Uint64(data[66:74])
	copy(h.TransactionsHash[:], data[74:106])
	copy(h.TargetUpdateHash[:], data[106:138])
	copy(h.Miner[:], data[138:170])
	h.MiningNonce = binary.BigEndian.Uint64(data[170:178])

	body := data[HeaderSize+1:]
	switch data[HeaderSize] {
	case 0:
	case 1:
		if len(body) < TargetUpdateSize+4 {
			return b, errors.New("truncated target update")
		}
		update, err := DecodeTargetUpdate(body[:TargetUpdateSize])
		if err != nil {
			return b, err
		}
		b.Update = &update
		body = body[TargetUpdateSize:]
	default:
		return b, errors.New("invalid target update flag")
	}
	count := binary.BigEndian.Uint32(body[:4])
	body = body[4:]
	if count > MaxTransfers || len(body) < int(count)*180 {
		return b, errors.New("invalid transfer count")
	}
	for i := uint32(0); i < count; i++ {
		if len(body) < 180 {
			return b, errors.New("truncated transfer")
		}
		size := 180 + int(binary.BigEndian.Uint16(body[114:116]))
		if size > len(body) {
			return b, errors.New("truncated memo or signature")
		}
		tx, err := DecodeTransfer(body[:size])
		if err != nil {
			return b, fmt.Errorf("transfer %d: %w", i, err)
		}
		b.Transfers = append(b.Transfers, tx)
		body = body[size:]
	}
	if len(body) != 0 {
		return b, errors.New("trailing block bytes")
	}
	return b, nil
}
