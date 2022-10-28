package structs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

const (
	SlotsPerEpoch   Slot = 32
	DurationPerSlot      = time.Second * 12
)

type PubKey struct{ types.PublicKey }

func (pk PubKey) Loggable() map[string]any {
	return map[string]any{
		"pubkey": pk,
	}
}

func (pk PubKey) Bytes() []byte {
	return pk.PublicKey[:]
}

func (pk PubKey) ValidatorKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk))
}

func (pk PubKey) RegistrationKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("registration-%s", pk))
}

type HeaderRequest map[string]string

func (hr HeaderRequest) Slot() (Slot, error) {
	slot, err := strconv.Atoi(hr["slot"])
	return Slot(slot), err
}

func (hr HeaderRequest) ParentHash() (types.Hash, error) {
	var parentHash types.Hash
	err := parentHash.UnmarshalText([]byte(strings.ToLower(hr["parent_hash"])))
	return parentHash, err
}

func (hr HeaderRequest) Pubkey() (PubKey, error) {
	var pk PubKey
	if err := pk.UnmarshalText([]byte(strings.ToLower(hr["pubkey"]))); err != nil {
		return PubKey{}, fmt.Errorf("invalid public key")
	}
	return pk, nil
}

type Slot uint64

func (s Slot) Loggable() map[string]any {
	return map[string]any{
		"slot":  s,
		"epoch": s.Epoch(),
	}
}

func (s Slot) Epoch() Epoch {
	return Epoch(s / SlotsPerEpoch)
}

func (s Slot) HeaderKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", s))
}

func (s Slot) PayloadKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%d", s))
}

type Epoch uint64

func (e Epoch) Loggable() map[string]any {
	return map[string]any{
		"epoch": e,
	}
}

type TraceQuery struct {
	Slot          Slot
	BlockHash     types.Hash
	BlockNum      uint64
	Pubkey        types.PublicKey
	Cursor, Limit uint64
}

func (q TraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q TraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q TraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q TraceQuery) HasPubkey() bool {
	return q.Pubkey != types.PublicKey{}
}

func (q TraceQuery) HasCursor() bool {
	return q.Cursor != 0
}

func (q TraceQuery) HasLimit() bool {
	return q.Limit != 0
}

type BidTraceExtended struct {
	types.BidTrace
	BlockNumber uint64 `json:"block_number,string"`
	NumTx       uint64 `json:"num_tx,string"`
}

type BidTraceWithTimestamp struct {
	BidTraceExtended
	Timestamp uint64 `json:"timestamp,string"`
}

type HeaderAndTrace struct {
	Header *types.ExecutionPayloadHeader
	Trace  *BidTraceWithTimestamp
}

type BlockBidAndTrace struct {
	Trace   *types.SignedBidTrace
	Bid     *types.GetHeaderResponse
	Payload *types.GetPayloadResponse
}

type DeliveredTrace struct {
	Trace       BidTraceWithTimestamp
	BlockNumber uint64
}
type PayloadKey struct {
	BlockHash types.Hash
	Proposer  types.PublicKey
	Slot      Slot
}

func PayloadKeyKey(key PayloadKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot))
}
