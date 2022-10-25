package structs

// SyncStatusPayload is the response payload for /eth/v1/node/syncing
type SyncStatusPayload struct {
	Data SyncStatusPayloadData
}

type SyncStatusPayloadData struct {
	HeadSlot  uint64 `json:"head_slot,string"`
	IsSyncing bool   `json:"is_syncing"`
}

// HeadEvent is emitted when subscribing to head events
type HeadEvent struct {
	Slot  uint64 `json:"slot,string"`
	Block string `json:"block"`
	State string `json:"state"`
}

func (h HeadEvent) Loggable() map[string]any {
	return map[string]any{
		"slot":  h.Slot,
		"block": h.Block,
		"state": h.State,
	}
}

// RegisteredProposersResponse is the response for querying proposer duties
type RegisteredProposersResponse struct {
	Data []RegisteredProposersResponseData
}

type RegisteredProposersResponseData struct {
	PubKey PubKey `json:"pubkey"`
	Slot   uint64 `json:"slot,string"`
}

// AllValidatorsResponse is the response for querying active validators
type AllValidatorsResponse struct {
	Data []ValidatorResponseEntry
}

type ValidatorResponseEntry struct {
	Index     uint64                         `json:"index,string"` // Index of validator in validator registry.
	Balance   string                         `json:"balance"`      // Current validator balance in gwei.
	Status    string                         `json:"status"`
	Validator ValidatorResponseValidatorData `json:"validator"`
}

type ValidatorResponseValidatorData struct {
	Pubkey string `json:"pubkey"`
}
