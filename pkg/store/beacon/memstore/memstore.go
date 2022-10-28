package memstore

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type BeaconMemstore struct {
	headSlot uint64

	updateTimeUnix int64

	proposerDutiesResponse []types.BuilderGetValidatorsResponseEntry
	pDMutex                sync.RWMutex

	knownValidatorsByIndex map[uint64]types.PubkeyHex
	knownValidators        map[types.PubkeyHex]struct{}
	kvMutex                sync.RWMutex
}

func NewBeaconMemstore() *BeaconMemstore {
	return &BeaconMemstore{}
}

func (bms *BeaconMemstore) SetKnownValidators(knownValidators map[types.PubkeyHex]struct{}, knownValidatorsByIndex map[uint64]types.PubkeyHex) {
	bms.kvMutex.Lock()
	defer bms.kvMutex.Unlock()
	bms.knownValidatorsByIndex = knownValidatorsByIndex
	bms.knownValidators = knownValidators
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

func (bms *BeaconMemstore) SetProposerDutiesResponse(vrsep []types.BuilderGetValidatorsResponseEntry) {
	bms.pDMutex.Lock()
	bms.proposerDutiesResponse = vrsep
	defer bms.pDMutex.Unlock()
}

func (bms *BeaconMemstore) ValidatorsMap() []types.BuilderGetValidatorsResponseEntry {
	bms.pDMutex.RLock()
	defer bms.pDMutex.RUnlock()
	return bms.proposerDutiesResponse
}
