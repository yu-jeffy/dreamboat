//go:generate mockgen  -destination=./mocks/datastore.go -package=mocks github.com/blocknative/dreamboat/pkg/relay RelayDatastore

package relay

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/lthibault/log"
	"golang.org/x/exp/constraints"
	"golang.org/x/sync/errgroup"
)

var (
	ErrNoPayloadFound = errors.New("no payload found")

	UnregisteredValidatorMsg = "unregistered validator"
	noBuilderBidMsg          = "no builder bid"
	badHeaderMsg             = "invalid block header from datastore"
)

type RelayDatastore interface {
	PutHeader(context.Context, structs.Slot, structs.HeaderAndTrace, time.Duration) error
	GetHeaders(ctx context.Context, query structs.TraceQuery) ([]structs.HeaderAndTrace, error)
	GetHeadersBySlot(context.Context, structs.Slot) ([]structs.HeaderAndTrace, error)
	GetHeadersBySlots(context.Context, []structs.Slot) ([]structs.HeaderAndTrace, error)
	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error
	GetDelivered(ctx context.Context, query structs.TraceQuery) (structs.BidTraceWithTimestamp, error)
	GetDeliveredBySlot(context.Context, structs.Slot) (structs.BidTraceWithTimestamp, error)
	GetDeliveredBySlots(context.Context, []structs.Slot) ([]structs.BidTraceWithTimestamp, error)
	PutPayload(context.Context, ds.Key, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, ds.Key) (*structs.BlockBidAndTrace, error)
	PutRegistration(context.Context, structs.PubKey, types.SignedValidatorRegistration, time.Duration) error
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}

type RelayConfig struct {
	TTL       time.Duration
	PubKey    types.PublicKey
	SecretKey *bls.SecretKey
}

type Relay struct {
	config RelayConfig
	l      log.Logger

	store RelayDatastore

	builderSigningDomain  types.Domain
	proposerSigningDomain types.Domain
}

// NewRelay relay service
func NewRelay(config RelayConfig, l log.Logger, store RelayDatastore, domainBuilder, domainBeaconProposer types.Domain) (*Relay, error) {
	return &Relay{
		l:      l,
		config: config,
		store:  store,

		builderSigningDomain:  domainBuilder,
		proposerSigningDomain: domainBeaconProposer,
	}, nil
}

// verifyTimestamp ensures timestamp is not too far in the future
func verifyTimestamp(timestamp uint64) bool {
	return timestamp > uint64(time.Now().Add(10*time.Second).Unix())
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Relay) RegisterValidator(ctx context.Context, headSlot structs.Slot, payload []structs.CheckedSignedValidatorRegistration) error {
	logger := rs.l.WithField("method", "RegisterValidator")
	timeStart := time.Now()

	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < runtime.NumCPU(); i++ {
		start := i * (len(payload) / runtime.NumCPU())
		end := (i + 1) * (len(payload) / runtime.NumCPU())
		if i == runtime.NumCPU()-1 {
			end = len(payload)
		}
		g.Go(func() error {
			return rs.processValidator(ctx, headSlot, payload[start:end])
		})
	}

	if err := g.Wait(); err != nil {
		logger.
			WithError(err).
			Debug("validator registration failed")
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"numberValidators": len(payload),
	}).Trace("validator registrations succeeded")

	return nil
}

func (rs *Relay) processValidator(ctx context.Context, headslot structs.Slot, payload []structs.CheckedSignedValidatorRegistration) error {
	logger := rs.l.WithField("method", "RegisterValidator")
	timeStart := time.Now()

	for i := 0; i < len(payload) && ctx.Err() == nil; i++ {
		registerRequest := payload[i]
		ok, err := types.VerifySignature(
			registerRequest.Message,
			rs.builderSigningDomain,
			registerRequest.Message.Pubkey[:],
			registerRequest.Signature[:],
		)
		if !ok || err != nil {
			logger.
				WithError(err).
				WithField("pubkey", registerRequest.Message.Pubkey).
				Debug("signature invalid")
			return fmt.Errorf("signature invalid for %s", registerRequest.Message.Pubkey.String())
		}

		if verifyTimestamp(registerRequest.Message.Timestamp) {
			return fmt.Errorf("request too far in future for %s", registerRequest.Message.Pubkey.String())
		}
		/*
			pk := structs.PubKey{registerRequest.Message.Pubkey}
			ok, err = rs.bstate.IsKnownValidator(pk.PubkeyHex()) // TODO(l): to be moved upwards
			if err != nil {
				return err
			} else if !ok {
				if rs.config.CheckKnownValidator {
					return fmt.Errorf("%s not a known validator", registerRequest.Message.Pubkey.String())
				}
				logger.
					WithField("pubkey", pk.PublicKey).
					WithField("slot", headslot).
					Debug("not a known validator")

			}
		*/
		// check previous validator registration
		pk := structs.PubKey{registerRequest.Message.Pubkey}
		previousValidator, err := rs.store.GetRegistration(ctx, pk)
		if err != nil && !errors.Is(err, ds.ErrNotFound) {
			logger.Warn(err)
		}

		if err == nil {
			// skip registration if
			if registerRequest.Message.Timestamp < previousValidator.Message.Timestamp {
				logger.WithField("pubkey", registerRequest.Message.Pubkey).Debug("request timestamp less than previous")
				continue
			}

			if registerRequest.Message.Timestamp == previousValidator.Message.Timestamp &&
				(registerRequest.Message.FeeRecipient != previousValidator.Message.FeeRecipient ||
					registerRequest.Message.GasLimit != previousValidator.Message.GasLimit) {
				// to help debug issues with validator set ups
				logger.With(log.F{
					"prevFeeRecipient":    previousValidator.Message.FeeRecipient,
					"requestFeeRecipient": registerRequest.Message.FeeRecipient,
					"prevGasLimit":        previousValidator.Message.GasLimit,
					"requestGasLimit":     registerRequest.Message.GasLimit,
					"pubkey":              registerRequest.Message.Pubkey,
				}).Debug("different registration fields")
			}
		}

		// officially register validator
		if err := rs.store.PutRegistration(ctx, pk, registerRequest.SignedValidatorRegistration, rs.config.TTL); err != nil {
			logger.WithField("pubkey", registerRequest.Message.Pubkey).WithError(err).Debug("Error in PutRegistration")
			return fmt.Errorf("failed to store %s", registerRequest.Message.Pubkey.String())
		}
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"numberValidators": len(payload),
	}).Trace("validator batch registered")

	return nil
}

// GetHeader is called by a block proposer communicating through mev-boost and returns a bid along with an execution payload header
func (rs *Relay) GetHeader(ctx context.Context, request structs.HeaderRequest) (*types.GetHeaderResponse, error) {
	logger := rs.l.WithField("method", "GetHeader")
	timeStart := time.Now()

	slot, err := request.Slot()
	if err != nil {
		return nil, err
	}

	parentHash, err := request.ParentHash()
	if err != nil {
		return nil, err
	}

	pk, err := request.Pubkey()
	if err != nil {
		return nil, err
	}

	logger = logger.With(log.F{
		"slot":       slot,
		"parentHash": parentHash,
		"pubkey":     pk,
	})

	logger.Trace("header requested")

	vd, err := rs.store.GetRegistration(ctx, pk)
	if err != nil {
		logger.Warn("unregistered validator")
		return nil, fmt.Errorf(noBuilderBidMsg)
	}
	if vd.Message.Pubkey != pk.PublicKey {
		logger.Warn("registration and request pubkey mismatch")
		return nil, fmt.Errorf("unknown validator")
	}

	headers, err := rs.store.GetHeadersBySlot(ctx, slot)
	if err != nil || len(headers) < 1 {
		logger.Warn(noBuilderBidMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

	header := headers[len(headers)-1] // choose the received last header

	if header.Header == nil || (header.Header.ParentHash != parentHash) {
		log.Debug(badHeaderMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

	bid := types.BuilderBid{
		Header: header.Header,
		Value:  header.Trace.Value,
		Pubkey: rs.config.PubKey,
	}

	signature, err := types.SignMessage(&bid, rs.builderSigningDomain, rs.config.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("internal server error")
	}

	response := &types.GetHeaderResponse{
		Version: "bellatrix",
		Data:    &types.SignedBuilderBid{Message: &bid, Signature: signature},
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"bidValue":         bid.Value.String(),
		"blockHash":        bid.Header.BlockHash.String(),
		"feeRecipient":     bid.Header.FeeRecipient.String(),
		"slot":             slot,
	}).Trace("bid sent")

	return response, nil
}

// GetPayload is called by a block proposer communicating through mev-boost and reveals execution payload of given signed beacon block if stored
func (rs *Relay) GetPayload(ctx context.Context, validatorPublicKey types.PublicKey, payloadRequest *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) {
	logger := rs.l.WithField("method", "GetPayload")
	timeStart := time.Now()

	logger.With(log.F{
		"slot":      payloadRequest.Message.Slot,
		"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		"pubkey":    validatorPublicKey,
	}).Debug("payload requested")

	ok, err := types.VerifySignature(
		payloadRequest.Message,
		rs.proposerSigningDomain,
		validatorPublicKey[:],
		payloadRequest.Signature[:],
	)
	if !ok || err != nil {
		logger.WithField(
			"pubkey", validatorPublicKey,
		).Error("signature invalid")
		return nil, fmt.Errorf("signature invalid")
	}

	payload, err := rs.store.GetPayload(ctx, structs.PayloadKeyKey(structs.PayloadKey{
		BlockHash: payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  validatorPublicKey,
		Slot:      structs.Slot(payloadRequest.Message.Slot),
	}))

	if err != nil || payload == nil {
		logger.WithError(err).With(log.F{
			"pubkey":    validatorPublicKey,
			"slot":      payloadRequest.Message.Slot,
			"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		}).Error("no payload found")
		return nil, ErrNoPayloadFound
	}

	logger.With(log.F{
		"slot":         payloadRequest.Message.Slot,
		"blockHash":    payload.Payload.Data.BlockHash,
		"blockNumber":  payload.Payload.Data.BlockNumber,
		"stateRoot":    payload.Payload.Data.StateRoot,
		"feeRecipient": payload.Payload.Data.FeeRecipient,
		"bid":          payload.Bid.Data.Message.Value,
		"numTx":        len(payload.Payload.Data.Transactions),
	}).Info("payload fetched")

	response := types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    payload.Payload.Data,
	}

	trace := structs.DeliveredTrace{
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 payloadRequest.Message.Slot,
					ParentHash:           payload.Payload.Data.ParentHash,
					BlockHash:            payload.Payload.Data.BlockHash,
					BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: payload.Payload.Data.FeeRecipient,
					GasLimit:             payload.Payload.Data.GasLimit,
					GasUsed:              payload.Payload.Data.GasUsed,
					Value:                payload.Trace.Message.Value,
				},
				BlockNumber: payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(payload.Payload.Data.Transactions)),
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
		BlockNumber: payload.Payload.Data.BlockNumber,
	}

	if err := rs.store.PutDelivered(ctx, structs.Slot(payloadRequest.Message.Slot), trace, rs.config.TTL); err != nil {
		rs.l.WithError(err).Warn("failed to set payload after delivery")
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"slot":             payloadRequest.Message.Slot,
		"blockHash":        payload.Payload.Data.BlockHash,
		"bid":              payload.Bid.Data.Message.Value,
	}).Trace("payload sent")

	return &response, nil
}

// SubmitBlock Accepts block from trusted builder and stores
func (rs *Relay) SubmitBlock(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	timeStart := time.Now()

	logger := rs.l.With(log.F{
		"method":    "SubmitBlock",
		"builder":   submitBlockRequest.Message.BuilderPubkey,
		"blockHash": submitBlockRequest.ExecutionPayload.BlockHash,
		"slot":      submitBlockRequest.Message.Slot,
		"proposer":  submitBlockRequest.Message.ProposerPubkey,
		"bid":       submitBlockRequest.Message.Value.String(),
	})

	logger.Trace("block submission requested")

	_, err := rs.verifyBlock(submitBlockRequest)
	if err != nil {
		logger.WithError(err).
			WithField("slot", submitBlockRequest.Message.Slot).
			WithField("builder", submitBlockRequest.Message.BuilderPubkey).
			Debug("block verification failed")
		return fmt.Errorf("verify block: %w", err)
	}

	signedBuilderBid, err := structs.SubmitBlockRequestToSignedBuilderBid(
		submitBlockRequest,
		rs.config.SecretKey,
		&rs.config.PubKey,
		rs.builderSigningDomain,
	)

	if err != nil {
		logger.WithError(err).
			With(log.F{
				"slot":    submitBlockRequest.Message.Slot,
				"builder": submitBlockRequest.Message.BuilderPubkey,
			}).Debug("signature failed")

		return fmt.Errorf("block submission failed: %w", err)
	}

	slot := structs.Slot(submitBlockRequest.Message.Slot)
	_, err = rs.store.GetDeliveredBySlot(ctx, slot)
	if err == nil {
		logger.Debug("block submission after payload delivered")
		return errors.New("the slot payload was already delivered")
	}

	payload := structs.SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitBlockRequest)

	submissionKey := structs.PayloadKeyKey(structs.PayloadKey{
		BlockHash: submitBlockRequest.ExecutionPayload.BlockHash,
		Proposer:  submitBlockRequest.Message.ProposerPubkey,
		Slot:      structs.Slot(submitBlockRequest.Message.Slot),
	})
	if err := rs.store.PutPayload(ctx, submissionKey, &payload, rs.config.TTL); err != nil {
		return err
	}

	header, err := types.PayloadToPayloadHeader(submitBlockRequest.ExecutionPayload)
	if err != nil {
		return err
	}

	h := structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 submitBlockRequest.Message.Slot,
					ParentHash:           payload.Payload.Data.ParentHash,
					BlockHash:            payload.Payload.Data.BlockHash,
					BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: payload.Payload.Data.FeeRecipient,
					Value:                submitBlockRequest.Message.Value,
					GasLimit:             payload.Trace.Message.GasLimit,
					GasUsed:              payload.Trace.Message.GasUsed,
				},
				BlockNumber: payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(payload.Payload.Data.Transactions)),
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
	}

	if err = rs.store.PutHeader(ctx, slot, h, rs.config.TTL); err != nil {
		logger.WithError(err).Error("PutHeader failed")
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Trace("builder block stored")
	return nil
}

func (rs *Relay) verifyBlock(SubmitBlockRequest *types.BuilderSubmitBlockRequest) (bool, error) {
	if SubmitBlockRequest == nil {
		return false, fmt.Errorf("block empty")
	}

	_ = simulateBlock()

	return types.VerifySignature(SubmitBlockRequest.Message, rs.builderSigningDomain, SubmitBlockRequest.Message.BuilderPubkey[:], SubmitBlockRequest.Signature[:])
}

func simulateBlock() bool {
	// TODO : Simulate block here once support for external builders
	// we currently only support a single internally trusted builder
	return true
}

func (rs *Relay) GetPayloadDelivered(ctx context.Context, headSlot structs.Slot, query structs.TraceQuery) ([]structs.BidTraceExtended, error) {
	var (
		event structs.BidTraceWithTimestamp
		err   error
	)

	if query.HasSlot() {
		event, err = rs.store.GetDeliveredBySlot(ctx, query.Slot)
	} else if query.HasBlockHash() {
		event, err = rs.store.GetDelivered(ctx, structs.TraceQuery{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		event, err = rs.store.GetDelivered(ctx, structs.TraceQuery{BlockNum: query.BlockNum})
	} else if query.HasPubkey() {
		event, err = rs.store.GetDelivered(ctx, structs.TraceQuery{Pubkey: query.Pubkey})
	} else {
		return rs.getTailDelivered(ctx, headSlot, query.Limit, query.Cursor, rs.config.TTL)
	}

	if err == nil {
		return []structs.BidTraceExtended{{BidTrace: event.BidTrace}}, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceExtended{}, nil
	}
	return nil, err
}

func (rs *Relay) getTailDelivered(ctx context.Context, headSlot structs.Slot, limit, cursor uint64, ttl time.Duration) ([]structs.BidTraceExtended, error) {
	start := headSlot
	if cursor != 0 {
		start = structs.Slot(cursor)
		if headSlot < structs.Slot(cursor) {
			start = headSlot
		}
	}

	stop := start - structs.Slot(ttl/structs.DurationPerSlot)

	batch := make([]structs.BidTraceWithTimestamp, 0, limit)
	//queries := make([]Query, 0, limit)
	slots := make([]structs.Slot, 0, limit)

	rs.l.WithField("limit", limit).
		WithField("start", start).
		WithField("stop", stop).
		Debug("querying delivered payload traces")

	for highSlot := start; len(batch) < int(limit) && stop <= highSlot; highSlot -= structs.Slot(limit) {
		slots = slots[:0]
		for s := highSlot; highSlot-structs.Slot(limit) < s && stop <= s; s-- {
			//queries = append(queries, Query{Slot: s})
			slots = append(slots, s)
		}

		nextBatch, err := rs.store.GetDeliveredBySlots(ctx, slots)
		if err != nil {
			rs.l.WithError(err).Warn("failed getting header batch")
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

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func (rs *Relay) GetBlockReceived(ctx context.Context, headSlot structs.Slot, query structs.TraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	var (
		events []structs.HeaderAndTrace
		err    error
	)

	if query.HasSlot() {
		events, err = rs.store.GetHeadersBySlot(ctx, query.Slot)
	} else if query.HasBlockHash() {
		events, err = rs.store.GetHeaders(ctx, structs.TraceQuery{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		events, err = rs.store.GetHeaders(ctx, structs.TraceQuery{BlockNum: query.BlockNum})
	} else {
		return rs.getTailBlockReceived(ctx, headSlot, query.Limit, rs.config.TTL)
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

func (rs *Relay) getTailBlockReceived(ctx context.Context, headslot structs.Slot, limit uint64, ttl time.Duration) ([]structs.BidTraceWithTimestamp, error) {
	batch := make([]structs.HeaderAndTrace, 0, limit)
	stop := headslot - structs.Slot(ttl/structs.DurationPerSlot)

	rs.l.WithField("limit", limit).
		WithField("start", headslot).
		WithField("stop", stop).
		Debug("querying received block traces")

	slots := make([]structs.Slot, 0)
	for highSlot := headslot; len(batch) < int(limit) && stop <= highSlot; highSlot -= structs.Slot(limit) {
		slots = slots[:0]
		for s := highSlot; highSlot-structs.Slot(limit) < s && stop <= s; s-- {
			slots = append(slots, s) // TODO(l): check validity
		}

		nextBatch, err := rs.store.GetHeadersBySlots(ctx, slots)
		if err != nil {
			rs.l.WithError(err).Warn("failed getting header batch")
			continue
		}
		batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
	}

	events := make([]structs.BidTraceWithTimestamp, 0, len(batch))
	for _, event := range batch {
		events = append(events, *event.Trace)
	}
	return events, nil
}

func (s *Relay) Registration(ctx context.Context, pk structs.PubKey) (types.SignedValidatorRegistration, error) {
	return s.store.GetRegistration(ctx, pk)
}
