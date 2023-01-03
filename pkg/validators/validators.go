package validators

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/verify"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
)

type State interface {
	Beacon() *structs.BeaconState
}

type Verifier interface {
	GetVerifyChan(stack uint) chan verify.Request
}

type RegistrationStore interface {
	PutRegistrationRaw(context.Context, structs.PubKey, []byte, time.Duration) error
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}

type RegistrationManager interface {
	SendStore(sReq StoreReq)
	//Get(k string) (value uint64, ok bool)
	Get(pubkey string) (timestamp uint64, ok bool)
}

type Register struct {
	d RegistrationStore

	l log.Logger

	beaconState State

	regMngr              RegistrationManager
	ver                  Verifier
	builderSigningDomain types.Domain
	m                    *RegisterMetrics
}

func NewRegister(l log.Logger, builderSigningDomain types.Domain, beaconState State, ver Verifier, d RegistrationStore) *Register {
	return &Register{
		d:                    d,
		l:                    l,
		ver:                  ver,
		builderSigningDomain: builderSigningDomain,
		beaconState:          beaconState,
	}
}

func (r *Register) Registration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	return r.d.GetRegistration(ctx, structs.PubKey{PublicKey: pk})
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Register) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) (err error) {
	logger := rs.l.WithField("method", "RegisterValidator")

	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "all"))
	defer timer.ObserveDuration()

	be := rs.beaconState.Beacon()

	verifyChan := rs.ver.GetVerifyChan(verify.ResponseQueueRegister)
	response := verify.NewRespC(len(payload))

	timeStart := time.Now()

	var totalCheckTime time.Duration
SendPayloads:
	for i, p := range payload {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			response.Close(0, err)
			return err
		default:
		}

		checkTime := time.Now()
		o, ok := verifyOther(be, rs.regMngr, i, p)
		if !ok {
			response.Close(i, o.Err)
			break SendPayloads
		}
		totalCheckTime += time.Since(checkTime)

		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.builderSigningDomain)
		if err != nil {
			response.Close(i, errors.New("invalid signature"))
			break SendPayloads
		}

		verifyChan <- verify.Request{
			Signature: p.Signature,
			Pubkey:    p.Message.Pubkey,
			Msg:       msg,
			ID:        i,
			Response:  response}
	}

	select {
	case <-response.Done():
	case <-ctx.Done():
		err := ctx.Err()
		response.Close(0, err)
		return err
	}
	processTime := time.Since(timeStart)
	rs.m.Timing.WithLabelValues("registerValidator", "verify").Observe(math.Abs(processTime.Seconds() - totalCheckTime.Seconds()))
	rs.m.Timing.WithLabelValues("registerValidator", "check").Observe(totalCheckTime.Seconds())

	if si := response.SuccessfullIndexes(); len(si) > 0 {
		timerStore := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "asyncStore"))
		request := StoreReq{Items: make([]StoreReqItem, len(si))}
		for nextIter, i := range si {
			p := payload[i]
			request.Items[nextIter] = StoreReqItem{
				Time:       p.Message.Timestamp,
				Pubkey:     p.Message.Pubkey,
				RawPayload: p.Raw,
			}
		}
		rs.regMngr.SendStore(request)
		timerStore.ObserveDuration()
	}

	err = response.Error()
	if err == nil {
		logger.
			WithField("processingTimeMs", time.Since(timeStart).Milliseconds()).
			WithField("numberValidators", len(payload)).
			Trace("validator registrations succeeded")
	}

	return err
}

func verifyOther(beacon *structs.BeaconState, tsReg RegistrationManager, i int, sp structs.SignedValidatorRegistration) (svresp verify.Resp, ok bool) {
	if verifyTimestamp(sp.Message.Timestamp) {
		return verify.Resp{Commit: false, ID: i, Err: fmt.Errorf("request too far in future for %s", sp.Message.Pubkey.String())}, false
	}

	pk := structs.PubKey{PublicKey: sp.Message.Pubkey}
	known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
	if !known {
		return verify.Resp{Commit: false, ID: i, Err: fmt.Errorf("%s not a known validator", sp.Message.Pubkey.String())}, false
	}

	previousValidatorTimestamp, ok := tsReg.Get(pk.String()) // Do not error on this
	return verify.Resp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), ID: i}, true
}

// verifyTimestamp ensures timestamp is not too far in the future
func verifyTimestamp(timestamp uint64) bool {
	return timestamp > uint64(time.Now().Add(10*time.Second).Unix())
}

// GetValidators returns a list of registered block proposers in current and next epoch
func (rs *Register) GetValidators() structs.BuilderGetValidatorsResponseEntrySlice {
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getValidators", "all"))
	defer timer.ObserveDuration()

	validators := rs.beaconState.Beacon().ValidatorsMap()
	return validators
}
