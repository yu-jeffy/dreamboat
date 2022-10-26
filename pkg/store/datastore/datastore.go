//go:generate mockgen -source=datastore.go -destination=../internal/mock/pkg/datastore.go -package=mock_relay
package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

type TTLStorage interface {
	PutWithTTL(context.Context, ds.Key, []byte, time.Duration) error
	Get(context.Context, ds.Key) ([]byte, error)
	Close() error
}

type Query struct {
	Slot      structs.Slot
	BlockHash types.Hash
	BlockNum  uint64
	PubKey    types.PublicKey
}

type Datastore struct {
	s  TTLStorage
	mu *sync.RWMutex
}

func NewDatastore(s TTLStorage) *Datastore {
	return &Datastore{s: s, mu: &sync.RWMutex{}}
}

func (s *Datastore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.s.Close()
}

func (s *Datastore) PutHeader(ctx context.Context, slot structs.Slot, header structs.HeaderAndTrace, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	headers, err := s.getHeaders(ctx, HeaderKey(slot))
	if errors.Is(err, ds.ErrNotFound) {
		headers = make([]structs.HeaderAndTrace, 0, 1)
	} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
		return err
	}

	if 0 < len(headers) && headers[len(headers)-1].Header.BlockHash == header.Header.BlockHash {
		return nil // deduplicate
	}

	headers = append(headers, header)

	if err := s.s.PutWithTTL(ctx, HeaderHashKey(header.Header.BlockHash), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.s.PutWithTTL(ctx, HeaderNumKey(header.Header.BlockNumber), HeaderKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	return s.s.PutWithTTL(ctx, HeaderKey(slot), data, ttl)
}

func (s *Datastore) GetHeaders(ctx context.Context, query Query) ([]structs.HeaderAndTrace, error) {
	key, err := s.queryToHeaderKey(ctx, query)
	if err != nil {
		return nil, err
	}
	headers, err := s.getHeaders(ctx, key)
	if err != nil {
		return nil, err
	}

	return s.deduplicateHeaders(headers, query), nil
}

func (s *Datastore) GetHeadersBySlot(ctx context.Context, slot structs.Slot) ([]structs.HeaderAndTrace, error) {
	return s.GetHeaders(ctx, Query{Slot: slot})
}

func (s *Datastore) getHeaders(ctx context.Context, key ds.Key) ([]structs.HeaderAndTrace, error) {
	s.mu.RLock()
	data, err := s.s.Get(ctx, key)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	return s.unsmarshalHeaders(data)
}

func (s *Datastore) deduplicateHeaders(headers []structs.HeaderAndTrace, query Query) []structs.HeaderAndTrace {
	filtered := headers[:0]
	for _, header := range headers {
		if (query.BlockHash != types.Hash{}) && (query.BlockHash != header.Header.BlockHash) {
			continue
		}
		if (query.BlockNum != 0) && (query.BlockNum != header.Header.BlockNumber) {
			continue
		}
		if (query.Slot != 0) && (uint64(query.Slot) != header.Trace.Slot) {
			continue
		}
		if (query.PubKey != types.PublicKey{}) && (query.PubKey != header.Trace.ProposerPubkey) {
			continue
		}
		filtered = append(filtered, header)
	}

	return filtered
}

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, trace structs.DeliveredTrace, ttl time.Duration) error {
	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.s.PutWithTTL(ctx, DeliveredHashKey(trace.Trace.BlockHash), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.s.PutWithTTL(ctx, DeliveredNumKey(trace.BlockNumber), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	if err := s.s.PutWithTTL(ctx, DeliveredPubkeyKey(trace.Trace.ProposerPubkey), DeliveredKey(slot).Bytes(), ttl); err != nil {
		return err
	}

	return s.s.PutWithTTL(ctx, DeliveredKey(slot), data, ttl)
}

func (s *Datastore) GetDelivered(ctx context.Context, query Query) (structs.BidTraceWithTimestamp, error) {
	key, err := s.queryToDeliveredKey(ctx, query)
	if err != nil {
		return structs.BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, key)
}

func (s *Datastore) GetDeliveredBySlot(ctx context.Context, slot structs.Slot) (structs.BidTraceWithTimestamp, error) {
	return s.GetDelivered(ctx, Query{Slot: slot})
}

func (s *Datastore) getDelivered(ctx context.Context, key ds.Key) (structs.BidTraceWithTimestamp, error) {
	s.mu.RLock()
	data, err := s.s.Get(ctx, key)
	s.mu.RUnlock()
	if err != nil {
		return structs.BidTraceWithTimestamp{}, err
	}

	var trace structs.BidTraceWithTimestamp
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s *Datastore) GetDeliveredBySlots(ctx context.Context, slots []structs.Slot) ([]structs.BidTraceWithTimestamp, error) {
	keys := make([]ds.Key, 0, len(slots))
	for _, sl := range slots {
		keys = append(keys, DeliveredKey(sl)) // TODO(l): check correctness
	}

	batch, err := s.getBatch(ctx, keys)
	if err != nil {
		return nil, err
	}

	traceBatch := make([]structs.BidTraceWithTimestamp, 0, len(batch))
	for _, data := range batch {
		var trace structs.BidTraceWithTimestamp
		if err = json.Unmarshal(data, &trace); err != nil {
			return nil, err
		}
		traceBatch = append(traceBatch, trace)
	}

	return traceBatch, err
}

/*
	func (s *Datastore) GetDeliveredBatch(ctx context.Context, queries []Query) ([]structs.BidTraceWithTimestamp, error) {
		keys := make([]ds.Key, 0, len(queries))
		for _, query := range queries {
			key, err := s.queryToDeliveredKey(ctx, query)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}

		batch, err := s.s.GetBatch(ctx, keys)
		if err != nil {
			return nil, err
		}

		traceBatch := make([]structs.BidTraceWithTimestamp, 0, len(batch))
		for _, data := range batch {
			var trace structs.BidTraceWithTimestamp
			if err = json.Unmarshal(data, &trace); err != nil {
				return nil, err
			}
			traceBatch = append(traceBatch, trace)
		}

		return traceBatch, err
	}
*/

func (s *Datastore) GetHeadersBySlots(ctx context.Context, slots []structs.Slot) ([]structs.HeaderAndTrace, error) {
	var batch []structs.HeaderAndTrace

	for _, slot := range slots {
		headers, err := s.getHeaders(ctx, HeaderKey(slot))
		if errors.Is(err, ds.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}

		batch = append(batch, headers...)
	}

	return batch, nil
}

/*
	func (s *Datastore) GetHeaderBatch(ctx context.Context, queries []Query) ([]structs.HeaderAndTrace, error) {
		var batch []structs.HeaderAndTrace

		for _, query := range queries {
			key, err := s.queryToHeaderKey(ctx, query)
			if err != nil {
				return nil, err
			}

			headers, err := s.getHeaders(ctx, key)
			if errors.Is(err, ds.ErrNotFound) {
				continue
			} else if err != nil {
				return nil, err
			}

			batch = append(batch, headers...)
		}

		return batch, nil
	}
*/
func (s *Datastore) unsmarshalHeaders(data []byte) ([]structs.HeaderAndTrace, error) {
	var headers []structs.HeaderAndTrace
	if err := json.Unmarshal(data, &headers); err != nil {
		var header structs.HeaderAndTrace
		if err := json.Unmarshal(data, &header); err != nil {
			return nil, err
		}
		return []structs.HeaderAndTrace{header}, nil
	}
	return headers, nil
}

func (s *Datastore) PutPayload(ctx context.Context, key ds.Key, payload *structs.BlockBidAndTrace, ttl time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.PutWithTTL(ctx, key, data, ttl)
}

func (s *Datastore) GetPayload(ctx context.Context, key ds.Key) (*structs.BlockBidAndTrace, error) {
	s.mu.RLock()
	data, err := s.s.Get(ctx, key)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	var payload structs.BlockBidAndTrace
	err = json.Unmarshal(data, &payload)
	return &payload, err
}

func (s *Datastore) PutRegistration(ctx context.Context, pk structs.PubKey, registration types.SignedValidatorRegistration, ttl time.Duration) error {
	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.PutWithTTL(ctx, RegistrationKey(pk), data, ttl)
}

func (s *Datastore) GetRegistration(ctx context.Context, pk structs.PubKey) (types.SignedValidatorRegistration, error) {
	s.mu.RLock()
	data, err := s.s.Get(ctx, RegistrationKey(pk))
	s.mu.RUnlock()
	if err != nil {
		return types.SignedValidatorRegistration{}, err
	}
	var registration types.SignedValidatorRegistration
	err = json.Unmarshal(data, &registration)
	return registration, err
}

func (s *Datastore) queryToHeaderKey(ctx context.Context, query Query) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		s.mu.RLock()
		rawKey, err = s.s.Get(ctx, HeaderHashKey(query.BlockHash))
		s.mu.RUnlock()
	} else if query.BlockNum != 0 {
		s.mu.RLock()
		rawKey, err = s.s.Get(ctx, HeaderNumKey(query.BlockNum))
		s.mu.RUnlock()
	} else {
		rawKey = HeaderKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}

func (s *Datastore) queryToDeliveredKey(ctx context.Context, query Query) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		s.mu.RLock()
		rawKey, err = s.s.Get(ctx, DeliveredHashKey(query.BlockHash))
		s.mu.RUnlock()
	} else if query.BlockNum != 0 {
		s.mu.RLock()
		rawKey, err = s.s.Get(ctx, DeliveredNumKey(query.BlockNum))
		s.mu.RUnlock()
	} else if (query.PubKey != types.PublicKey{}) {
		s.mu.RLock()
		rawKey, err = s.s.Get(ctx, DeliveredPubkeyKey(query.PubKey))
		s.mu.RUnlock()
	} else {
		rawKey = DeliveredKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}

func HeaderKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", slot))
}

func HeaderHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-hash-%s", bh.String()))
}

func HeaderNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("header-num-%d", bn))
}

func DeliveredKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-%d", slot))
}

func DeliveredHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-hash-%s", bh.String()))
}

func DeliveredNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-num-%d", bn))
}

func DeliveredPubkeyKey(pk types.PublicKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-pk-%s", pk.String()))
}

func ValidatorKey(pk structs.PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk.String()))
}

func RegistrationKey(pk structs.PubKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("registration-%s", pk.String()))
}

func (s *Datastore) getBatch(ctx context.Context, keys []ds.Key) (batch [][]byte, err error) {
	for _, key := range keys {
		data, err := s.s.Get(ctx, key)
		if err != nil {
			continue
		}
		batch = append(batch, data)
	}

	return
}
