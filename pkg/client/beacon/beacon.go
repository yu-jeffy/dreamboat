package beacon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/lthibault/log"
	"github.com/r3labs/sse/v2"
)

var (
	ErrBeaconNodeSyncing = errors.New("beacon node is syncing")
)

var (
	ErrHTTPErrorResponse = errors.New("got an HTTP error response")
	ErrNodesUnavailable  = errors.New("beacon nodes are unavailable")
)

type BeaconClient struct {
	beaconEndpoint *url.URL
	log            log.Logger
}

func NewBeaconClient(endpoint string, l log.Logger) (*BeaconClient, error) {
	u, err := url.Parse(endpoint)

	bc := &BeaconClient{
		beaconEndpoint: u,
		log:            l.WithField("beaconEndpoint", endpoint),
	}

	return bc, err
}

/*
func (b *BeaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	logger := b.log.WithField("method", "SubscribeToHeadEvents")

	eventsURL := fmt.Sprintf("%s/eth/v1/events?topics=head", b.beaconEndpoint.String())

	go func() {
		defer logger.Debug("head events subscription stopped")

		for {
			client := sse.NewClient(eventsURL)
			err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
				var head HeadEvent
				if err := json.Unmarshal(msg.Data, &head); err != nil {
					logger.WithError(err).Debug("event subscription failed")
				}

				select {
				case <-ctx.Done():
					return
				case slotC <- head:
					logger.
						With(head).
						Debug("read head subscription")
				}
			})

			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}

			logger.WithError(err).Debug("beacon subscription failed, restarting...")
		}
	}()
}*/

func (b *BeaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan structs.HeadEvent) {
	logger := b.log.WithField("method", "SubscribeToHeadEvents")

	eventsURL := fmt.Sprintf("%s/eth/v1/events?topics=head", b.beaconEndpoint.String())

	defer logger.Debug("head events subscription stopped")

	for {
		client := sse.NewClient(eventsURL)
		err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
			var head structs.HeadEvent
			if err := json.Unmarshal(msg.Data, &head); err != nil {
				logger.WithError(err).Debug("event subscription failed")
			}

			select {
			case <-ctx.Done():
				return
			case slotC <- head:
				logger.
					With(head).
					Debug("read head subscription")
			}
		})

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}

		logger.WithError(err).Debug("beacon subscription failed, restarting...")
	}
}

// Returns proposer duties for every slot in this epoch
func (b *BeaconClient) GetProposerDuties(epoch structs.Epoch) (*structs.RegisteredProposersResponse, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/Validator/getProposerDuties
	u.Path = fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch)
	resp := &structs.RegisteredProposersResponse{}
	err := b.queryBeacon(&u, "GET", resp)
	return resp, err
}

// SyncStatus returns the current node sync-status
func (b *BeaconClient) SyncStatus() (*structs.SyncStatusPayloadData, error) {
	u := *b.beaconEndpoint
	// https://ethereum.github.io/beacon-APIs/#/ValidatorRequiredApi/getSyncingStatus
	u.Path = "/eth/v1/node/syncing"
	resp := &structs.SyncStatusPayload{}
	err := b.queryBeacon(&u, "GET", resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (b *BeaconClient) KnownValidators(headSlot structs.Slot) (structs.AllValidatorsResponse, error) {
	u := *b.beaconEndpoint
	u.Path = fmt.Sprintf("/eth/v1/beacon/states/%d/validators", headSlot)
	q := u.Query()
	q.Add("status", "active,pending")
	u.RawQuery = q.Encode()

	var vd structs.AllValidatorsResponse
	err := b.queryBeacon(&u, "GET", &vd)
	return vd, err
}

func (b *BeaconClient) Endpoint() string {
	return b.beaconEndpoint.String()
}

func (b *BeaconClient) queryBeacon(u *url.URL, method string, dst any) error {
	logger := b.log.
		WithField("method", "QueryBeacon").
		WithField("URL", u.RequestURI())
	timeStart := time.Now()

	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return fmt.Errorf("invalid request for %s: %w", u, err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("client refused for %s: %w", u, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body for %s: %w", u, err)
	}

	if resp.StatusCode >= 300 {
		ec := &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{}
		if err = json.Unmarshal(bodyBytes, ec); err != nil {
			return fmt.Errorf("could not unmarshal error response from beacon node for %s from %s: %w", u, string(bodyBytes), err)
		}
		return fmt.Errorf("%w: %s", ErrHTTPErrorResponse, ec.Message)
	}

	err = json.Unmarshal(bodyBytes, dst)
	if err != nil {
		return fmt.Errorf("could not unmarshal response for %s from %s: %w", u, string(bodyBytes), err)
	}

	logger.
		WithField("processingTimeMs", time.Since(timeStart).Milliseconds()).
		WithField("bytesAmount", len(bodyBytes)).
		Debug("beacon queried")

	return nil
}
