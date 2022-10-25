package beacon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
)

type BeaconClient interface {
	SubscribeToHeadEvents(ctx context.Context, slotC chan structs.HeadEvent)
	GetProposerDuties(structs.Epoch) (*structs.RegisteredProposersResponse, error)
	SyncStatus() (*structs.SyncStatusPayloadData, error)
	KnownValidators(structs.Slot) (structs.AllValidatorsResponse, error)
	Endpoint() string
}

type BeaconState interface {
	SetKnownValidator(pubkey types.PubkeyHex, index uint64)
	KnownValidatorByIndex(index uint64) types.PubkeyHex
	IsKnownValidator(pk types.PubkeyHex) bool
	SetHeadSlot(sl structs.Slot)
	HeadSlot() structs.Slot

	SetUpdateTime(unixTime int64)
	UpdateTime() time.Time
}

type BeaconManager struct {
	l      log.Logger
	client BeaconClient

	memoryStore BeaconState
}

func NewBeaconManager(l log.Logger, client BeaconClient) *BeaconManager {
	return &BeaconManager{l: l, client: client}
}

func (bm *BeaconManager) BeaconEventLoop(ctx context.Context) error {
	syncStatus, err := bm.client.SyncStatus()
	if err != nil {
		return err
	}
	if syncStatus.IsSyncing {
		return ErrBeaconNodeSyncing
	}

	err = bm.updateProposerDuties(ctx, structs.Slot(syncStatus.HeadSlot))
	if err != nil {
		return err
	}

	defer bm.l.Debug("beacon loop stopped")

	events := make(chan structs.HeadEvent)

	go bm.client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if err := bm.processNewSlot(ctx, ev); err != nil {
				bm.l.
					With(ev).
					WithError(err).
					Warn("error processing slot")
				continue
			}
		}
	}
}

func (bm *BeaconManager) processNewSlot(ctx context.Context, event structs.HeadEvent) error {
	logger := bm.l.WithField("method", "ProcessNewSlot")
	timeStart := time.Now()

	received := structs.Slot(event.Slot)
	if received <= bm.headslotSlot {
		return nil
	}

	if bm.headslotSlot > 0 {
		for slot := bm.headslotSlot + 1; slot < received; slot++ {
			bm.l.Warnf("missedSlot %d", slot)
		}
	}

	//bm.headslotSlot = received

	logger.With(log.F{
		"epoch":              received.Epoch(),
		"slotHead":           received,
		"slotStartNextEpoch": structs.Slot(received.Epoch()+1) * structs.SlotsPerEpoch,
	},
	).Debugf("updated headSlot to %d", received)

	// update proposer duties and known validators in the background
	if (DurationPerEpoch / 2) < time.Since(bm.memoryStore.UpdateTime()) { // only update every half DurationPerEpoch
		go func() {
			if err := bm.updateKnownValidators(ctx, received); err != nil {
				bm.l.WithError(err).Warn("failed to update known validators")
			} else {
				bm.memoryStore.SetUpdateTime(time.Now().Unix())
				s.setReady()
			}
		}()
	}

	if err := bm.updateProposerDuties(ctx, received); err != nil {
		return err
	}

	logger.With(log.F{
		"epoch":              received.Epoch(),
		"slotHead":           received,
		"slotStartNextEpoch": structs.Slot(received.Epoch()+1) * structs.SlotsPerEpoch,
		"slot":               uint64(received),
		"processingTimeMs":   time.Since(timeStart).Milliseconds(),
	}).Info("updated head slot")

	return nil
}

func (bm *BeaconManager) updateProposerDuties(ctx context.Context, headSlot structs.Slot) error {
	epoch := headSlot.Epoch()

	logger := bm.l.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	timeStart := time.Now()

	state := dutiesState{}

	// Query current epoch
	current, err := bm.client.GetProposerDuties(epoch)
	if err != nil {
		return fmt.Errorf("current epoch: get proposer duties: %w", err)
	}

	entries := current.Data

	// Query next epoch
	next, err := bm.client.GetProposerDuties(epoch + 1)
	if err != nil {
		return fmt.Errorf("next epoch: get proposer duties: %w", err)
	}
	entries = append(entries, next.Data...)

	state.proposerDutiesResponse = make([]types.BuilderGetValidatorsResponseEntry, 0, len(entries))
	state.currentSlot = headSlot

	for _, e := range entries {
		reg, err := bm.store.GetRegistration(ctx, e.PubKey)
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

func (bm *BeaconManager) updateKnownValidators(ctx context.Context, current structs.Slot) error {
	logger := bm.l.WithField("method", "UpdateKnownValidators")
	timeStart := time.Now()

	//state := validatorsState{}
	validators, err := bm.client.KnownValidators(current)
	if err != nil {
		return err
	}

	for _, vs := range validators.Data {
		bm.memoryStore.SetKnownValidator(types.NewPubkeyHex(vs.Validator.Pubkey), vs.Index)
	}
	/*
		knownValidators := make(map[types.PubkeyHex]struct{})
		knownValidatorsByIndex := make(map[uint64]types.PubkeyHex)
		for _, vs := range validators.Data {
			knownValidators[types.NewPubkeyHex(vs.Validator.Pubkey)] = struct{}{}
			knownValidatorsByIndex[vs.Index] = types.NewPubkeyHex(vs.Validator.Pubkey)
		}

		state.knownValidators = knownValidators
		state.knownValidatorsByIndex = knownValidatorsByIndex

		s.state.validators.Store(state)
	*/
	logger.With(log.F{
		"slotHead":         uint64(current),
		"numValidators":    len(knownValidators),
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Debug("updated known validators")

	return nil
}

type BeaconMemstore struct {
	headSlot uint64

	updateTimeUnix int64

	proposerDutiesResponse []types.BuilderGetValidatorsResponseEntry

	knownValidatorsByIndex map[uint64]types.PubkeyHex
	knownValidators        map[types.PubkeyHex]struct{}
	kvMutex                sync.RWMutex
}

func NewBeaconMemstore() *BeaconMemstore {
	return &BeaconMemstore{}
}

func (bms *BeaconMemstore) SetKnownValidator(pubkey types.PubkeyHex, index uint64) {
	bms.kvMutex.Lock()
	defer bms.kvMutex.Unlock()
	bms.knownValidatorsByIndex[index] = pubkey
	bms.knownValidators[pubkey] = struct{}{}
}

func (bms *BeaconMemstore) ResetKnownValidators(pubkey types.PubkeyHex, index uint64) {
	bms.kvMutex.Lock()
	defer bms.kvMutex.Unlock()
	bms.knownValidatorsByIndex[index] = pubkey
	bms.knownValidators[pubkey] = struct{}{}
}

func (bms *BeaconMemstore) KnownValidatorByIndex(index uint64) types.PubkeyHex {
	bms.kvMutex.RLock()
	defer bms.kvMutex.RUnlock()
	return bms.knownValidatorsByIndex[index]
}

func (bms *BeaconMemstore) IsKnownValidator(pk types.PubkeyHex) bool {
	bms.kvMutex.RLock()
	defer bms.kvMutex.RUnlock()
	_, ok := bms.knownValidators[pk]
	return ok
}

func (bms *BeaconMemstore) SetHeadSlot(sl structs.Slot) {
	atomic.StoreUint64(&bms.headSlot, uint64(sl))
}

func (bms *BeaconMemstore) SetUpdateTime(unixTime int64) {
	atomic.StoreInt64(&bms.updateTimeUnix, unixTime)
}

func (bms *BeaconMemstore) UpdateTime() time.Time {
	return time.Unix(bms.updateTimeUnix, 0)
}

func (bms *BeaconMemstore) HeadSlot() structs.Slot {
	return structs.Slot(bms.headSlot)
}

func (bms *BeaconMemstore) ValidatorsMap() []types.BuilderGetValidatorsResponseEntry {
	//return
}

/*

type DefaultService struct {
	Log log.Logger

	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	// state
	state        atomicState
	headslotSlot structs.Slot
	updateTime   atomic.Value
}


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
*/

/*

type atomicState struct {
	duties     atomic.Value
	validators atomic.Value
}

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

*/
