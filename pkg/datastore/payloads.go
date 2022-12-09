package datastore

import (
	"context"
	"encoding/json"

	"github.com/blocknative/dreamboat/pkg/structs"
)

func (s *Datastore) PutSentHeader(ctx context.Context, payload *structs.BlockBidAndTrace) error {
	key := structs.PayloadKey{BlockHash: payload.Trace.Message.BlockHash, Proposer: payload.Trace.Message.ProposerPubkey, Slot: structs.Slot(payload.Trace.Message.Slot)}
	s.payloadCache.Add(key, payload)
	return nil
}

func (s *Datastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockBidAndTrace, error) {
	if payload, ok := s.payloadCache.Get(key); ok {
		return payload, nil
	}

	data, err := s.TTLStorage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, err
	}
	var payload structs.BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, err
}
