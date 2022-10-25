package beacon

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/lthibault/log"
)

type MultiBeaconClient struct {
	Log     log.Logger
	Clients []BeaconClient

	bestBeaconIndex int64
}

func NewMultiBeaconClient(l log.Logger, clients []BeaconClient) BeaconClient {
	return &MultiBeaconClient{Log: l.WithField("service", "multi-beacon client"), Clients: clients}
}

func (b *MultiBeaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan structs.HeadEvent) {
	for _, client := range b.Clients {
		go client.SubscribeToHeadEvents(ctx, slotC)
	}
}

func (b *MultiBeaconClient) GetProposerDuties(epoch structs.Epoch) (*structs.RegisteredProposersResponse, error) {
	// return the first successful beacon node response
	clients := b.clientsByLastResponse()

	for i, client := range clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		duties, err := client.GetProposerDuties(epoch)
		if err != nil {
			log.WithError(err).Error("failed to get proposer duties")
			continue
		}

		atomic.StoreInt64(&b.bestBeaconIndex, int64(i))

		// Received successful response. Set this index as last successful beacon node
		return duties, nil
	}

	return nil, ErrNodesUnavailable
}

func (b *MultiBeaconClient) SyncStatus() (*structs.SyncStatusPayloadData, error) {
	var bestSyncStatus *structs.SyncStatusPayloadData
	var foundSyncedNode bool

	// Check each beacon-node sync status
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, instance := range b.Clients {
		wg.Add(1)
		go func(client BeaconClient) {
			defer wg.Done()
			log := b.Log.WithField("endpoint", client.Endpoint())

			syncStatus, err := client.SyncStatus()
			if err != nil {
				log.WithError(err).Error("failed to get sync status")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			if foundSyncedNode {
				return
			}

			if bestSyncStatus == nil {
				bestSyncStatus = syncStatus
			}

			if !syncStatus.IsSyncing {
				bestSyncStatus = syncStatus
				foundSyncedNode = true
			}
		}(instance)
	}

	// Wait for all requests to complete...
	wg.Wait()

	if !foundSyncedNode {
		return nil, ErrBeaconNodeSyncing
	}

	if bestSyncStatus == nil {
		return nil, ErrNodesUnavailable
	}

	return bestSyncStatus, nil
}

func (b *MultiBeaconClient) KnownValidators(headSlot structs.Slot) (a structs.AllValidatorsResponse, err error) {
	// return the first successful beacon node response
	clients := b.clientsByLastResponse()

	for i, client := range clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		validators, err := client.KnownValidators(headSlot)
		if err != nil {
			log.WithError(err).Error("failed to fetch validators")
			continue
		}

		atomic.StoreInt64(&b.bestBeaconIndex, int64(i))

		// Received successful response. Set this index as last successful beacon node
		return validators, nil
	}

	return a, ErrNodesUnavailable
}

func (b *MultiBeaconClient) Endpoint() string {
	return b.clientsByLastResponse()[0].Endpoint()
}

// beaconInstancesByLastResponse returns a list of beacon clients that has the client
// with the last successful response as the first element of the slice
func (b *MultiBeaconClient) clientsByLastResponse() []BeaconClient {
	if b.bestBeaconIndex == 0 {
		return b.Clients
	}

	instances := make([]BeaconClient, len(b.Clients))
	copy(instances, b.Clients)
	instances[0], instances[b.bestBeaconIndex] = instances[b.bestBeaconIndex], instances[0]

	return instances
}
