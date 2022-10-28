package beacon

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
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

func TestMultiSubscribeToHeadEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	connected := mock_relay.NewMockBeaconClient(ctrl)
	disconnected := mock_relay.NewMockBeaconClient(ctrl)

	bc := &relay.MultiBeaconClient{Log: nullLog, Clients: []relay.BeaconClient{connected, disconnected}}

	events := make(chan relay.HeadEvent)

	connected.EXPECT().SubscribeToHeadEvents(ctx, events).Times(1)
	disconnected.EXPECT().SubscribeToHeadEvents(ctx, events).Times(1)

	bc.SubscribeToHeadEvents(ctx, events)

	time.Sleep(time.Second) // wait for goroutines to spawn
}

func TestMultiGetProposerDuties(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("connected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mock_relay.NewMockBeaconClient(ctrl)
		disconnected := mock_relay.NewMockBeaconClient(ctrl)

		clients := []relay.BeaconClient{connected, disconnected}

		bc := &relay.MultiBeaconClient{Log: nullLog, Clients: clients}

		duties := relay.RegisteredProposersResponse{}

		epoch := structs.Epoch(rand.Uint64())

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			GetProposerDuties(epoch).
			Return(&duties, nil).
			Times(1)

		gotDuties, err := bc.GetProposerDuties(epoch)
		require.NoError(t, err)

		require.Equal(t, &duties, gotDuties)
	})

	t.Run("disconnected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mock_relay.NewMockBeaconClient(ctrl)
		disconnected := mock_relay.NewMockBeaconClient(ctrl)

		clients := []relay.BeaconClient{disconnected, connected}

		bc := &relay.MultiBeaconClient{Log: nullLog, Clients: clients}

		duties := relay.RegisteredProposersResponse{}

		epoch := structs.Epoch(rand.Uint64())

		disconnected.EXPECT().Endpoint().Times(1)
		disconnected.EXPECT().
			GetProposerDuties(epoch).
			Return(nil, relay.ErrHTTPErrorResponse).
			Times(1)

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			GetProposerDuties(epoch).
			Return(&duties, nil).
			Times(1)

		gotDuties, err := bc.GetProposerDuties(epoch)
		require.NoError(t, err)

		require.Equal(t, &duties, gotDuties)
	})
}

func TestMultiSyncStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("all beacons connected", func(t *testing.T) {
		t.Parallel()

		connected := mock_relay.NewMockBeaconClient(ctrl)
		connected2 := mock_relay.NewMockBeaconClient(ctrl)

		clients := []relay.BeaconClient{connected, connected2}

		bc := &relay.MultiBeaconClient{Log: nullLog, Clients: clients}

		status := relay.SyncStatusPayloadData{}

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			SyncStatus().
			Return(&status, nil).
			Times(1)

		connected2.EXPECT().Endpoint().Times(1)
		connected2.EXPECT().
			SyncStatus().
			Return(&status, nil).
			Times(1)

		gotStatus, err := bc.SyncStatus()
		require.NoError(t, err)

		require.Equal(t, &status, gotStatus)
	})

	t.Run("with disconnected beacon", func(t *testing.T) {
		t.Parallel()

		connected := mock_relay.NewMockBeaconClient(ctrl)
		disconnected := mock_relay.NewMockBeaconClient(ctrl)

		clients := []relay.BeaconClient{disconnected, connected}

		bc := &relay.MultiBeaconClient{Log: nullLog, Clients: clients}

		status := relay.SyncStatusPayloadData{}

		disconnected.EXPECT().Endpoint().Times(1)
		disconnected.EXPECT().
			SyncStatus().
			Return(nil, relay.ErrHTTPErrorResponse).
			Times(1)

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			SyncStatus().
			Return(&status, nil).
			Times(1)

		gotStatus, err := bc.SyncStatus()
		require.NoError(t, err)

		require.Equal(t, &status, gotStatus)
	})
}

func TestMultiKnownValidators(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("connected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mock_relay.NewMockBeaconClient(ctrl)
		disconnected := mock_relay.NewMockBeaconClient(ctrl)

		clients := []relay.BeaconClient{connected, disconnected}

		bc := &relay.MultiBeaconClient{Log: nullLog, Clients: clients}

		validators := relay.AllValidatorsResponse{}

		slot := structs.Slot(rand.Uint64())

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			KnownValidators(slot).
			Return(validators, nil).
			Times(1)

		gotValidators, err := bc.KnownValidators(slot)
		require.NoError(t, err)

		require.Equal(t, validators, gotValidators)
	})

	t.Run("disconnected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mock_relay.NewMockBeaconClient(ctrl)
		disconnected := mock_relay.NewMockBeaconClient(ctrl)

		clients := []relay.BeaconClient{disconnected, connected}

		bc := &relay.MultiBeaconClient{Log: nullLog, Clients: clients}

		validators := relay.AllValidatorsResponse{Data: []relay.ValidatorResponseEntry{}}
		disconnectedValidators := relay.AllValidatorsResponse{Data: nil}

		slot := structs.Slot(rand.Uint64())

		disconnected.EXPECT().Endpoint().Times(1)
		disconnected.EXPECT().
			KnownValidators(slot).
			Return(disconnectedValidators, relay.ErrHTTPErrorResponse).
			Times(1)

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			KnownValidators(slot).
			Return(validators, nil).
			Times(1)

		gotValidators, err := bc.KnownValidators(slot)
		require.NoError(t, err)

		require.EqualValues(t, validators, gotValidators)
	})
}
