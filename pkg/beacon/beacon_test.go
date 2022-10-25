package beacon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestBeaconClientState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relayMock := mock_relay.NewMockRelay(ctrl)
	beaconMock := mock_relay.NewMockBeaconClient(ctrl)
	service := relay.DefaultService{
		Relay: relayMock,
		NewBeaconClient: func() (relay.BeaconClient, error) {
			return beaconMock, nil
		},
		Datastore: &relay.DefaultDatastore{TTLStorage: newMockDatastore()},
	}

	beaconMock.EXPECT().GetProposerDuties(gomock.Any()).Return(&relay.RegisteredProposersResponse{[]relay.RegisteredProposersResponseData{}}, nil).Times(4)
	beaconMock.EXPECT().SyncStatus().Return(&relay.SyncStatusPayloadData{}, nil).Times(1)

	beaconMock.EXPECT().SubscribeToHeadEvents(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(ctx context.Context, events chan relay.HeadEvent) {
			go func() {
				events <- relay.HeadEvent{Slot: 1}
			}()
		},
	)
	beaconMock.EXPECT().KnownValidators(gomock.Any()).Return(relay.AllValidatorsResponse{Data: []relay.ValidatorResponseEntry{}}, nil).Times(1)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := service.Run(ctx)
		require.Error(t, err, context.Canceled)
	}()
	<-service.Ready()

	relayMock.EXPECT().
		GetHeader(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1)

	service.GetHeader(nil, nil)
	time.Sleep(time.Second) // give time for the beacon state manager to kick-off

	cancel()
}
