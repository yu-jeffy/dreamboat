package beacon

import (
	"context"
	"errors"
	"fmt"
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

type BeaconManager struct {
	l      log.Logger
	client BeaconClient
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

	err = bm.updateProposerDuties(ctx, bm.client, structs.Slot(syncStatus.HeadSlot))
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

	bm.headslotSlot = received

	logger.With(log.F{
		"epoch":              s.headslotSlot.Epoch(),
		"slotHead":           s.headslotSlot,
		"slotStartNextEpoch": structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
	},
	).Debugf("updated headSlot to %d", received)

	// update proposer duties and known validators in the background
	if (DurationPerEpoch / 2) < time.Since(s.knownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func() {
			if err := bm.updateKnownValidators(ctx, s.headslotSlot); err != nil {
				s.l.WithError(err).Warn("failed to update known validators")
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

func (bm *BeaconManager) updateProposerDuties(ctx context.Context, client BeaconClient, headSlot structs.Slot) error {
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
		reg, err := s.store.GetRegistration(ctx, e.PubKey)
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

func (bm *BeaconManager) updateKnownValidators(ctx context.Context, client BeaconClient, current structs.Slot) error {
	logger := bm.l.WithField("method", "UpdateKnownValidators")
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

func (bm *BeaconManager) knownValidatorsUpdateTime() time.Time {
	updateTime, ok := s.updateTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return updateTime
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
