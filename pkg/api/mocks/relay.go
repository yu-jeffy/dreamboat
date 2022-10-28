// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/blocknative/dreamboat/pkg/api (interfaces: Relay,BeaconState)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	structs "github.com/blocknative/dreamboat/pkg/structs"
	types "github.com/flashbots/go-boost-utils/types"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockRelay is a mock of Relay interface
type MockRelay struct {
	ctrl     *gomock.Controller
	recorder *MockRelayMockRecorder
}

// MockRelayMockRecorder is the mock recorder for MockRelay
type MockRelayMockRecorder struct {
	mock *MockRelay
}

// NewMockRelay creates a new mock instance
func NewMockRelay(ctrl *gomock.Controller) *MockRelay {
	mock := &MockRelay{ctrl: ctrl}
	mock.recorder = &MockRelayMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockRelay) EXPECT() *MockRelayMockRecorder {
	return m.recorder
}

// GetBlockReceived mocks base method
func (m *MockRelay) GetBlockReceived(arg0 context.Context, arg1 structs.Slot, arg2 structs.TraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBlockReceived", arg0, arg1, arg2)
	ret0, _ := ret[0].([]structs.BidTraceWithTimestamp)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBlockReceived indicates an expected call of GetBlockReceived
func (mr *MockRelayMockRecorder) GetBlockReceived(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBlockReceived", reflect.TypeOf((*MockRelay)(nil).GetBlockReceived), arg0, arg1, arg2)
}

// GetHeader mocks base method
func (m *MockRelay) GetHeader(arg0 context.Context, arg1 structs.HeaderRequest) (*types.GetHeaderResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetHeader", arg0, arg1)
	ret0, _ := ret[0].(*types.GetHeaderResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetHeader indicates an expected call of GetHeader
func (mr *MockRelayMockRecorder) GetHeader(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetHeader", reflect.TypeOf((*MockRelay)(nil).GetHeader), arg0, arg1)
}

// GetPayload mocks base method
func (m *MockRelay) GetPayload(arg0 context.Context, arg1 types.PublicKey, arg2 *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPayload", arg0, arg1, arg2)
	ret0, _ := ret[0].(*types.GetPayloadResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPayload indicates an expected call of GetPayload
func (mr *MockRelayMockRecorder) GetPayload(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPayload", reflect.TypeOf((*MockRelay)(nil).GetPayload), arg0, arg1, arg2)
}

// GetPayloadDelivered mocks base method
func (m *MockRelay) GetPayloadDelivered(arg0 context.Context, arg1 structs.Slot, arg2 structs.TraceQuery) ([]structs.BidTraceExtended, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPayloadDelivered", arg0, arg1, arg2)
	ret0, _ := ret[0].([]structs.BidTraceExtended)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPayloadDelivered indicates an expected call of GetPayloadDelivered
func (mr *MockRelayMockRecorder) GetPayloadDelivered(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPayloadDelivered", reflect.TypeOf((*MockRelay)(nil).GetPayloadDelivered), arg0, arg1, arg2)
}

// RegisterValidator mocks base method
func (m *MockRelay) RegisterValidator(arg0 context.Context, arg1 structs.Slot, arg2 []structs.CheckedSignedValidatorRegistration) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterValidator", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// RegisterValidator indicates an expected call of RegisterValidator
func (mr *MockRelayMockRecorder) RegisterValidator(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterValidator", reflect.TypeOf((*MockRelay)(nil).RegisterValidator), arg0, arg1, arg2)
}

// Registration mocks base method
func (m *MockRelay) Registration(arg0 context.Context, arg1 structs.PubKey) (types.SignedValidatorRegistration, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Registration", arg0, arg1)
	ret0, _ := ret[0].(types.SignedValidatorRegistration)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Registration indicates an expected call of Registration
func (mr *MockRelayMockRecorder) Registration(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Registration", reflect.TypeOf((*MockRelay)(nil).Registration), arg0, arg1)
}

// SubmitBlock mocks base method
func (m *MockRelay) SubmitBlock(arg0 context.Context, arg1 *types.BuilderSubmitBlockRequest) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubmitBlock", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// SubmitBlock indicates an expected call of SubmitBlock
func (mr *MockRelayMockRecorder) SubmitBlock(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubmitBlock", reflect.TypeOf((*MockRelay)(nil).SubmitBlock), arg0, arg1)
}

// MockBeaconState is a mock of BeaconState interface
type MockBeaconState struct {
	ctrl     *gomock.Controller
	recorder *MockBeaconStateMockRecorder
}

// MockBeaconStateMockRecorder is the mock recorder for MockBeaconState
type MockBeaconStateMockRecorder struct {
	mock *MockBeaconState
}

// NewMockBeaconState creates a new mock instance
func NewMockBeaconState(ctrl *gomock.Controller) *MockBeaconState {
	mock := &MockBeaconState{ctrl: ctrl}
	mock.recorder = &MockBeaconStateMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockBeaconState) EXPECT() *MockBeaconStateMockRecorder {
	return m.recorder
}

// HeadSlot mocks base method
func (m *MockBeaconState) HeadSlot() structs.Slot {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HeadSlot")
	ret0, _ := ret[0].(structs.Slot)
	return ret0
}

// HeadSlot indicates an expected call of HeadSlot
func (mr *MockBeaconStateMockRecorder) HeadSlot() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HeadSlot", reflect.TypeOf((*MockBeaconState)(nil).HeadSlot))
}

// IsKnownValidator mocks base method
func (m *MockBeaconState) IsKnownValidator(arg0 types.PubkeyHex) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsKnownValidator", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsKnownValidator indicates an expected call of IsKnownValidator
func (mr *MockBeaconStateMockRecorder) IsKnownValidator(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsKnownValidator", reflect.TypeOf((*MockBeaconState)(nil).IsKnownValidator), arg0)
}

// KnownValidatorByIndex mocks base method
func (m *MockBeaconState) KnownValidatorByIndex(arg0 uint64) (types.PubkeyHex, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "KnownValidatorByIndex", arg0)
	ret0, _ := ret[0].(types.PubkeyHex)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// KnownValidatorByIndex indicates an expected call of KnownValidatorByIndex
func (mr *MockBeaconStateMockRecorder) KnownValidatorByIndex(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "KnownValidatorByIndex", reflect.TypeOf((*MockBeaconState)(nil).KnownValidatorByIndex), arg0)
}

// ValidatorsMap mocks base method
func (m *MockBeaconState) ValidatorsMap() []types.BuilderGetValidatorsResponseEntry {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidatorsMap")
	ret0, _ := ret[0].([]types.BuilderGetValidatorsResponseEntry)
	return ret0
}

// ValidatorsMap indicates an expected call of ValidatorsMap
func (mr *MockBeaconStateMockRecorder) ValidatorsMap() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidatorsMap", reflect.TypeOf((*MockBeaconState)(nil).ValidatorsMap))
}
