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

var (
	DurationPerEpoch = structs.DurationPerSlot * time.Duration(structs.SlotsPerEpoch)
)
var (
	ErrBeaconNodeSyncing = errors.New("beacon node is syncing")
)

type Datastore interface {
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}

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

	store Datastore

	beaconStore BeaconState
}

func NewBeaconManager(l log.Logger, beaconStore BeaconState, client BeaconClient) *BeaconManager {
	return &BeaconManager{l: l, beaconStore: beaconStore, client: client}
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
	var headslotSlot structs.Slot
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			recvSlot := structs.Slot(ev.Slot)
			if recvSlot <= headslotSlot {
				continue
			}

			if headslotSlot > 0 {
				for slot := headslotSlot + 1; slot < recvSlot; slot++ {
					bm.l.Warnf("missedSlot %d", slot)
				}
			}

			if err := bm.processNewSlot(ctx, (DurationPerEpoch / 2), recvSlot); err != nil {
				bm.l.
					With(ev).
					WithError(err).
					Warn("error processing slot")
				continue
			}
		}
	}
}

func (bm *BeaconManager) processNewSlot(ctx context.Context, updateProposerDuration time.Duration, received structs.Slot) error {

	logger := bm.l.WithField("method", "ProcessNewSlot")
	timeStart := time.Now()

	logger.With(log.F{
		"epoch":              received.Epoch(),
		"slotHead":           received,
		"slotStartNextEpoch": structs.Slot(received.Epoch()+1) * structs.SlotsPerEpoch,
	},
	).Debugf("updated headSlot to %d", received)

	// BUG(l): It will trigger multiple times
	// update proposer duties and known validators in the background
	if updateProposerDuration < time.Since(bm.beaconStore.UpdateTime()) { // only update every half DurationPerEpoch
		go func() {
			if err := bm.updateKnownValidators(ctx, received); err != nil {
				bm.l.WithError(err).Warn("failed to update known validators")
				return
			}
			bm.beaconStore.SetUpdateTime(time.Now().Unix())
			s.setReady()
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

	//state := dutiesState{}

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

	//state.proposerDutiesResponse = make([]types.BuilderGetValidatorsResponseEntry, 0, len(entries))
	pdr := make([]types.BuilderGetValidatorsResponseEntry, 0, len(entries))
	bm.beaconStore.SetHeadSlot(headSlot)

	for _, e := range entries {
		reg, err := bm.store.GetRegistration(ctx, e.PubKey)
		if err == nil {
			logger.With(e.PubKey).
				Debug("new proposer duty")

			pdr = append(pdr, types.BuilderGetValidatorsResponseEntry{
				Slot:  e.Slot,
				Entry: &reg,
			})
		} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
			logger.Warn(err)
		}
	}
	bm.beaconStore.HeadSlot(pdr)

	logger.With(log.F{
		"processingTimeMs":      time.Since(timeStart).Milliseconds(),
		"receivedDuties":        len(entries),
		"builderValidatorsKeys": pdr,
	}).Debug("proposer duties updated")

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
		bm.beaconStore.SetKnownValidator(types.NewPubkeyHex(vs.Validator.Pubkey), vs.Index)
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
