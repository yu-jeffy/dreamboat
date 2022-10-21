package relay

import (
	"golang.org/x/exp/constraints"
)

type UserAgent string

/*
	type GetValidatorRelayResponse []struct {
		Slot  uint64 `json:"slot,string"`
		Entry struct {
			Message struct {
				FeeRecipient string `json:"fee_recipient"`
				GasLimit     uint64 `json:"gas_limit,string"`
				Timestamp    uint64 `json:"timestamp,string"`
				PubKey       string `json:"pubkey"`
			} `json:"message"`
			Signature string `json:"signature"`
		} `json:"entry"`
	}
*/

type ErrBadProposer struct {
	Want, Got PubKey
}

func (ErrBadProposer) Error() string { return "peer is not proposer" }

func Max[T constraints.Ordered](args ...T) T {
	if len(args) == 0 {
		return *new(T) // zero value of T
	}

	if isNan(args[0]) {
		return args[0]
	}

	max := args[0]
	for _, arg := range args[1:] {

		if isNan(arg) {
			return arg
		}

		if arg > max {
			max = arg
		}
	}
	return max
}

func isNan[T constraints.Ordered](arg T) bool {
	return arg != arg
}
