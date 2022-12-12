package datastore

import (
	"context"
	"encoding/json"

	"github.com/blocknative/dreamboat/pkg/structs"
)

func (s *Datastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockBidAndTrace, error) {
	if payload, ok := s.payloadCache.Get(key); ok {
		s.Logger.With(key).Debug("payload cache hit")
		return payload, nil
	}
	s.Logger.With(key).Debug("payload cache miss")

	data, err := s.TTLStorage.Get(ctx, PayloadKeyKey(key))
	if err != nil {
		return nil, err
	}
	var payload structs.BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, err
}
