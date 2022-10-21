//go:generate mockgen -source=service.go -destination=../internal/mock/pkg/service.go -package=mock_relay
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/lthibault/log"
	"golang.org/x/exp/constraints"
)

type Datastore interface {
	PutHeader(context.Context, structs.Slot, structs.HeaderAndTrace, time.Duration) error
	GetHeaders(context.Context, Query) ([]structs.HeaderAndTrace, error)
	GetHeaderBatch(context.Context, []Query) ([]structs.HeaderAndTrace, error)
	PutDelivered(context.Context, structs.Slot, DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, Query) (structs.BidTraceWithTimestamp, error)
	GetDeliveredBatch(context.Context, []Query) ([]structs.BidTraceWithTimestamp, error)
	PutPayload(context.Context, PayloadKey, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, PayloadKey) (*structs.BlockBidAndTrace, error)
	PutRegistration(context.Context, structs.PubKey, types.SignedValidatorRegistration, time.Duration) error
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}

const (
	Version = "0.2.0"
)

/*
type BuilderGetValidatorsResponseEntrySlice []types.BuilderGetValidatorsResponseEntry

func (b BuilderGetValidatorsResponseEntrySlice) Loggable() map[string]any {
	return map[string]any{
		"numDuties": len(b),
	}
}*/

type Relay interface {
	// Proposer APIs
	RegisterValidator(context.Context, []types.SignedValidatorRegistration, State) error
	GetHeader(context.Context, structs.HeaderRequest, State) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock, State) (*types.GetPayloadResponse, error)

	// Builder APIs
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest, State) error
	GetValidators(State) []types.BuilderGetValidatorsResponseEntry
}

type BeaconClient interface {
	SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent)
	GetProposerDuties(structs.Epoch) (*RegisteredProposersResponse, error)
	SyncStatus() (*SyncStatusPayloadData, error)
	KnownValidators(structs.Slot) (AllValidatorsResponse, error)
	Endpoint() string
}

type DefaultService struct {
	Log log.Logger
	TTL time.Duration

	Relay           Relay
	Storage         TTLStorage
	Datastore       Datastore
	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	// state
	state        atomicState
	headslotSlot structs.Slot
	updateTime   atomic.Value
}

/*
// Run creates a relay, datastore and starts the beacon client event loop

	func (s *DefaultService) Run(ctx context.Context) (err error) {
		if s.Log == nil {
			s.Log = log.New().WithField("service", "RelayService")
		}

		timeRelayStart := time.Now()
		if s.Relay == nil {
			s.Relay, err = NewRelay(s.Config)
			if err != nil {
				return
			}
		}
		s.Log.WithFields(logrus.Fields{
			"service":     "relay",
			"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
		}).Info("initialized")

		timeDataStoreStart := time.Now()
		if s.Datastore == nil {
			if s.Storage == nil {
				storage, err := badger.NewDatastore(s.Config.Datadir, &badger.DefaultOptions)
				if err != nil {
					s.Log.WithError(err).Fatal("failed to initialize datastore")
					return err
				}
				s.Storage = &TTLDatastoreBatcher{storage}
			}

			s.Datastore = &datastore.Datastore{TTLStorage: s.Storage}
		}
		s.Log.
			WithFields(logrus.Fields{
				"service":     "datastore",
				"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
			}).Info("data store initialized")

		s.state.datastore.Store(s.Datastore)

		if s.NewBeaconClient == nil {
			s.NewBeaconClient = func() (BeaconClient, error) {
				clients := make([]BeaconClient, 0, len(s.Config.BeaconEndpoints))
				for _, endpoint := range s.Config.BeaconEndpoints {
					client, err := NewBeaconClient(endpoint, s.Config)
					if err != nil {
						return nil, err
					}
					clients = append(clients, client)
				}
				return NewMultiBeaconClient(s.Config.Log.WithField("service", "multi-beacon client"), clients), nil
			}
		}

		client, err := s.NewBeaconClient()
		if err != nil {
			s.Log.WithError(err).Warn("failed beacon client registration")
			return err
		}

		s.Log.Info("beacon client initialized")

		return s.beaconEventLoop(ctx, client)
	}
*/
func (s *DefaultService) Ready() <-chan struct{} {
	s.once.Do(func() {
		s.ready = make(chan struct{})
	})
	return s.ready
}

func (s *DefaultService) setReady() {
	select {
	case <-s.Ready():
	default:
		close(s.ready)
	}
}

func (s *DefaultService) beaconEventLoop(ctx context.Context, client BeaconClient) error {
	syncStatus, err := client.SyncStatus()
	if err != nil {
		return err
	}
	if syncStatus.IsSyncing {
		return ErrBeaconNodeSyncing
	}

	err = s.updateProposerDuties(ctx, client, structs.Slot(syncStatus.HeadSlot))
	if err != nil {
		return err
	}

	defer s.Log.Debug("beacon loop stopped")

	events := make(chan structs.HeadEvent)

	client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if err := s.processNewSlot(ctx, client, ev); err != nil {
				s.Log.
					With(ev).
					WithError(err).
					Warn("error processing slot")
				continue
			}
		}
	}
}

func (s *DefaultService) processNewSlot(ctx context.Context, client BeaconClient, event HeadEvent) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")
	timeStart := time.Now()

	received := structs.Slot(event.Slot)
	if received <= s.headslotSlot {
		return nil
	}

	if s.headslotSlot > 0 {
		for slot := s.headslotSlot + 1; slot < received; slot++ {
			s.Log.Warnf("missedSlot %d", slot)
		}
	}

	s.headslotSlot = received

	logger.With(log.F{
		"epoch":              s.headslotSlot.Epoch(),
		"slotHead":           s.headslotSlot,
		"slotStartNextEpoch": structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
	},
	).Debugf("updated headSlot to %d", received)

	// update proposer duties and known validators in the background
	if (DurationPerEpoch / 2) < time.Since(s.knownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func() {
			if err := s.updateKnownValidators(ctx, client, s.headslotSlot); err != nil {
				s.Log.WithError(err).Warn("failed to update known validators")
			} else {
				s.updateTime.Store(time.Now())
				s.setReady()
			}
		}()
	}

	if err := s.updateProposerDuties(ctx, client, s.headslotSlot); err != nil {
		return err
	}

	logger.With(log.F{
		"epoch":              s.headslotSlot.Epoch(),
		"slotHead":           s.headslotSlot,
		"slotStartNextEpoch": structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
		"slot":               uint64(s.headslotSlot),
		"processingTimeMs":   time.Since(timeStart).Milliseconds(),
	}).Info("updated head slot")

	return nil
}

func (s *DefaultService) knownValidatorsUpdateTime() time.Time {
	updateTime, ok := s.updateTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return updateTime
}

func (s *DefaultService) updateProposerDuties(ctx context.Context, client BeaconClient, headSlot structs.Slot) error {
	epoch := headSlot.Epoch()

	logger := s.Log.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	timeStart := time.Now()

	state := dutiesState{}

	// Query current epoch
	current, err := client.GetProposerDuties(epoch)
	if err != nil {
		return fmt.Errorf("current epoch: get proposer duties: %w", err)
	}

	entries := current.Data

	// Query next epoch
	next, err := client.GetProposerDuties(epoch + 1)
	if err != nil {
		return fmt.Errorf("next epoch: get proposer duties: %w", err)
	}
	entries = append(entries, next.Data...)

	state.proposerDutiesResponse = make([]types.BuilderGetValidatorsResponseEntry, 0, len(entries))
	state.currentSlot = headSlot

	for _, e := range entries {
		reg, err := s.Datastore.GetRegistration(ctx, e.PubKey)
		if err == nil {
			logger.With(e.PubKey).
				Debug("new proposer duty")

			state.proposerDutiesResponse = append(state.proposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
				Slot:  e.Slot,
				Entry: &reg,
			})
		} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
			logger.Warn(err)
		}
	}

	s.state.duties.Store(state)

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"receivedDuties":   len(entries),
	}).With(state.proposerDutiesResponse).Debug("proposer duties updated")

	return nil
}

func (s *DefaultService) updateKnownValidators(ctx context.Context, client BeaconClient, current structs.Slot) error {
	logger := s.Log.WithField("method", "UpdateKnownValidators")
	timeStart := time.Now()

	state := validatorsState{}
	validators, err := client.KnownValidators(current)
	if err != nil {
		return err
	}

	knownValidators := make(map[types.PubkeyHex]struct{})
	knownValidatorsByIndex := make(map[uint64]types.PubkeyHex)
	for _, vs := range validators.Data {
		knownValidators[types.NewPubkeyHex(vs.Validator.Pubkey)] = struct{}{}
		knownValidatorsByIndex[vs.Index] = types.NewPubkeyHex(vs.Validator.Pubkey)
	}

	state.knownValidators = knownValidators
	state.knownValidatorsByIndex = knownValidatorsByIndex

	s.state.validators.Store(state)

	logger.With(log.F{
		"slotHead":         uint64(current),
		"numValidators":    len(knownValidators),
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Debug("updated known validators")

	return nil
}

func (s *DefaultService) RegisterValidator(ctx context.Context, payload []types.SignedValidatorRegistration) error {
	return s.Relay.RegisterValidator(ctx, payload, &s.state)
}

func (s *DefaultService) GetHeader(ctx context.Context, request structs.HeaderRequest) (*types.GetHeaderResponse, error) {
	return s.Relay.GetHeader(ctx, request, &s.state)
}

func (s *DefaultService) GetPayload(ctx context.Context, payloadRequest *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) {
	return s.Relay.GetPayload(ctx, payloadRequest, &s.state)
}

func (s *DefaultService) SubmitBlock(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	return s.Relay.SubmitBlock(ctx, submitBlockRequest, &s.state)
}

func (s *DefaultService) GetValidators() []types.BuilderGetValidatorsResponseEntry {
	return s.Relay.GetValidators(&s.state)
}

func (s *DefaultService) GetPayloadDelivered(ctx context.Context, query structs.TraceQuery) ([]structs.BidTraceExtended, error) {
	var (
		event structs.BidTraceWithTimestamp
		err   error
	)

	if query.HasSlot() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{Slot: query.Slot})
	} else if query.HasBlockHash() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{BlockNum: query.BlockNum})
	} else if query.HasPubkey() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{PubKey: query.Pubkey})
	} else {
		return s.getTailDelivered(ctx, query.Limit, query.Cursor)
	}

	if err == nil {
		return []structs.BidTraceExtended{{BidTrace: event.BidTrace}}, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceExtended{}, nil
	}
	return nil, err
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func (s *DefaultService) getTailDelivered(ctx context.Context, limit, cursor uint64) ([]structs.BidTraceExtended, error) {
	headSlot := s.state.Beacon().HeadSlot()
	start := headSlot
	if cursor != 0 {
		start = min(headSlot, structs.Slot(cursor))
	}

	stop := start - structs.Slot(s.TTL/DurationPerSlot)

	batch := make([]structs.BidTraceWithTimestamp, 0, limit)
	queries := make([]Query, 0, limit)

	s.Log.WithField("limit", limit).
		WithField("start", start).
		WithField("stop", stop).
		Debug("querying delivered payload traces")

	for highSlot := start; len(batch) < int(limit) && stop <= highSlot; highSlot -= structs.Slot(limit) {
		queries = queries[:0]
		for s := highSlot; highSlot-structs.Slot(limit) < s && stop <= s; s-- {
			queries = append(queries, Query{Slot: s})
		}

		nextBatch, err := s.state.Datastore().GetDeliveredBatch(ctx, queries)
		if err != nil {
			s.Log.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]structs.BidTraceExtended, 0, len(batch))
	for _, event := range batch {
		events = append(events, event.BidTraceExtended)
	}
	return events, nil
}

func (s *DefaultService) GetBlockReceived(ctx context.Context, query structs.TraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	var (
		events []structs.HeaderAndTrace
		err    error
	)

	if query.HasSlot() {
		events, err = s.state.Datastore().GetHeaders(ctx, Query{Slot: query.Slot})
	} else if query.HasBlockHash() {
		events, err = s.state.Datastore().GetHeaders(ctx, Query{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		events, err = s.state.Datastore().GetHeaders(ctx, Query{BlockNum: query.BlockNum})
	} else {
		return s.getTailBlockReceived(ctx, query.Limit)
	}

	if err == nil {
		traces := make([]structs.BidTraceWithTimestamp, 0, len(events))
		for _, event := range events {
			traces = append(traces, *event.Trace)
		}
		return traces, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceWithTimestamp{}, nil
	}
	return nil, err
}

func (s *DefaultService) getTailBlockReceived(ctx context.Context, limit uint64) ([]structs.BidTraceWithTimestamp, error) {
	batch := make([]structs.HeaderAndTrace, 0, limit)
	stop := s.state.Beacon().HeadSlot() - structs.Slot(s.TTL/DurationPerSlot)
	queries := make([]Query, 0)

	s.Log.WithField("limit", limit).
		WithField("start", s.state.Beacon().HeadSlot()).
		WithField("stop", stop).
		Debug("querying received block traces")

	for highSlot := s.state.Beacon().HeadSlot(); len(batch) < int(limit) && stop <= highSlot; highSlot -= structs.Slot(limit) {
		queries = queries[:0]
		for s := highSlot; highSlot-structs.Slot(limit) < s && stop <= s; s-- {
			queries = append(queries, Query{Slot: s})
		}

		nextBatch, err := s.state.Datastore().GetHeaderBatch(ctx, queries)
		if err != nil {
			s.Log.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]structs.BidTraceWithTimestamp, 0, len(batch))
	for _, event := range batch {
		events = append(events, *event.Trace)
	}
	return events, nil
}

func (s *DefaultService) Registration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	return s.Datastore.GetRegistration(ctx, structs.PubKey{pk})
}

type atomicState struct {
	datastore  atomic.Value
	duties     atomic.Value
	validators atomic.Value
}

func (as *atomicState) Datastore() Datastore { return as.datastore.Load().(Datastore) }

func (as *atomicState) Beacon() BeaconState {
	duties := as.duties.Load().(dutiesState)
	validators := as.validators.Load().(validatorsState)
	return beaconState{dutiesState: duties, validatorsState: validators}
}

type beaconState struct {
	dutiesState
	validatorsState
}

func (s beaconState) KnownValidatorByIndex(index uint64) (types.PubkeyHex, error) {
	pk, ok := s.knownValidatorsByIndex[index]
	if !ok {
		return "", ErrUnknownValue
	}
	return pk, nil
}

func (s beaconState) IsKnownValidator(pk types.PubkeyHex) (bool, error) {
	_, ok := s.knownValidators[pk]
	return ok, nil
}

func (s beaconState) KnownValidators() map[types.PubkeyHex]struct{} {
	return s.knownValidators
}

func (s beaconState) HeadSlot() structs.Slot {
	return s.currentSlot
}

func (s beaconState) ValidatorsMap() []types.BuilderGetValidatorsResponseEntry {
	return s.proposerDutiesResponse
}

type dutiesState struct {
	currentSlot            structs.Slot
	proposerDutiesResponse []types.BuilderGetValidatorsResponseEntry
}

type validatorsState struct {
	knownValidatorsByIndex map[uint64]types.PubkeyHex
	knownValidators        map[types.PubkeyHex]struct{}
}
