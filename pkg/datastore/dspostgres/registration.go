package dspostgres

import (
	"context"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
)

func (s *Datastore) GetRegistration(ctx context.Context, pk structs.PubKey) (types.SignedValidatorRegistration, error) {
	row := s.Db.QueryRowContext(ctx, `SELECT signature, fee_recipient, gas_limit, reg_time FROM validator_registrations WHERE pubkey = $1 LIMIT 1;`, pk)

	reg := types.SignedValidatorRegistration{}
	msg := types.RegisterValidatorRequestMessage{}

	err := row.Scan(&reg.Signature, &msg.FeeRecipient, &msg.GasLimit, &msg.Timestamp, pk)
	return reg, err
}

/*
func (s *Datastore) PutRegistrationRaw(ctx context.Context, pk structs.PubKey, registration []byte, ttl time.Duration) error {

		//return // s.TTLStorage.PutWithTTL(ctx, RegistrationKey(pk), registration, ttl)
	}
*/
func (s *Datastore) PutRegistration(ctx context.Context, reg types.SignedValidatorRegistration, ttl time.Duration) error {
	_, err := s.Db.ExecContext(ctx, `INSERT INTO validator_registrations(pubkey, signature, fee_recipient, gas_limit, reg_time)
										VALUES ($1, $2, $3, $4, $5)
										ON CONFLICT (pubkey)
										DO UPDATE SET
										signature = EXCLUDED.signature,
										fee_recipient = EXCLUDED.fee_recipient,
										gas_limit = EXCLUDED.gas_limit,
										reg_time = EXCLUDED.reg_time,
										WHERE validator_registrations.reg_time < EXCLUDED.reg_time`,
		reg.Message.Pubkey, reg.Signature, reg.Message.FeeRecipient, reg.Message.GasLimit, reg.Message.Timestamp)
	return err
}
