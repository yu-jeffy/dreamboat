package api

import (
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/blocknative/dreamboat/pkg/api/mocks"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

var (
	logger = log.New(log.WithWriter(io.Discard))
)

func TestMuxRouting(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("Status", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathStatus, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.EqualValues(t, w.Code, http.StatusOK)
	})

	t.Run("RegisterValidator", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodPost, PathRegisterValidator, nil)
		w := httptest.NewRecorder()

		relay.EXPECT().
			RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		bDS.EXPECT().HeadSlot().
			Times(1)

		mux.ServeHTTP(w, req)
	})

	t.Run("GetHeader", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathGetHeader, nil)
		w := httptest.NewRecorder()

		relay.EXPECT().
			GetHeader(gomock.Any(), gomock.Any()).
			Times(1)

		mux.ServeHTTP(w, req)
	})

	t.Run("GetPayload", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathGetPayload, nil)
		w := httptest.NewRecorder()

		relay.EXPECT().
			GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		bDS.EXPECT().HeadSlot().
			Times(1)

		mux.ServeHTTP(w, req)
	})

	t.Run("SubmitBlock", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)
		req := httptest.NewRequest(http.MethodPost, PathSubmitBlock, nil)
		w := httptest.NewRecorder()

		relay.EXPECT().
			SubmitBlock(gomock.Any(), gomock.Any()).
			Times(1)

		mux.ServeHTTP(w, req)
	})

	t.Run("GetValidators", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathGetValidators, nil)
		w := httptest.NewRecorder()

		bDS.EXPECT().ValidatorsMap().
			Times(1)
		mux.ServeHTTP(w, req)
	})

	t.Run("builderBlocksReceived", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathBuilderBlocksReceived, nil)
		q := req.URL.Query()
		q.Add("slot", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)

		relay.EXPECT().
			GetBlockReceived(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, Slot: 100}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived block_hash", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		blockHash := types.Hash(random32Bytes())
		q.Add("block_hash", blockHash.String())
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetBlockReceived(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, BlockHash: blockHash}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived block_number", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		q.Add("block_number", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetBlockReceived(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, BlockNum: 100}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived limit", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		q.Add("limit", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetBlockReceived(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: 50}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived no limit", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetBlockReceived(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit}).
			Times(1)

		mux.ServeHTTP(w, req)
	})

	t.Run("payloadDelivered", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()
		q.Add("slot", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetPayloadDelivered(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, Slot: 100}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered block_hash", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		blockHash := types.Hash(random32Bytes())
		q.Add("block_hash", blockHash.String())
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetPayloadDelivered(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, BlockHash: blockHash}).
			Times(1)

		mux.ServeHTTP(w, req)
	})

	t.Run("payloadDelivered block_number", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("block_number", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetPayloadDelivered(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, BlockNum: 100}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered cursor", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("cursor", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetPayloadDelivered(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit, Cursor: 50}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered limit", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("limit", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetPayloadDelivered(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: 50}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered no limit", func(t *testing.T) {
		t.Parallel()

		relay := mocks.NewMockRelay(ctrl)
		bDS := mocks.NewMockBeaconState(ctrl)
		a := NewApi(logger, bDS, relay, true)
		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		req := httptest.NewRequest(http.MethodGet, PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		bDS.EXPECT().HeadSlot().
			Times(1)
		relay.EXPECT().
			GetPayloadDelivered(gomock.Any(), gomock.Any(), structs.TraceQuery{Limit: DataLimit}).
			Times(1)

		mux.ServeHTTP(w, req)
	})
}

var reqs = []*http.Request{
	httptest.NewRequest(http.MethodGet, PathStatus, nil),
	httptest.NewRequest(http.MethodPost, PathRegisterValidator, nil),
	httptest.NewRequest(http.MethodGet, PathGetHeader, nil),
	httptest.NewRequest(http.MethodGet, PathGetPayload, nil),
	httptest.NewRequest(http.MethodPost, PathSubmitBlock, nil),
	httptest.NewRequest(http.MethodGet, PathGetValidators, nil),
}

func BenchmarkAPISequential(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	relay := mocks.NewMockRelay(ctrl)
	bDS := mocks.NewMockBeaconState(ctrl)
	a := NewApi(logger, bDS, relay, true)
	mux := http.NewServeMux()
	a.AttachToHandler(mux)

	relay.EXPECT().GetHeader(gomock.Any(), gomock.Any()).AnyTimes()
	relay.EXPECT().GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	relay.EXPECT().RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	relay.EXPECT().SubmitBlock(gomock.Any(), gomock.Any()).AnyTimes()

	w := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mux.ServeHTTP(w, reqs[rand.Intn(len(reqs))])
	}
}

func BenchmarkAPIParallel(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	var wg sync.WaitGroup
	defer wg.Wait()

	relay := mocks.NewMockRelay(ctrl)
	bDS := mocks.NewMockBeaconState(ctrl)
	a := NewApi(logger, bDS, relay, true)
	mux := http.NewServeMux()
	a.AttachToHandler(mux)

	relay.EXPECT().GetHeader(gomock.Any(), gomock.Any()).AnyTimes()
	relay.EXPECT().GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	relay.EXPECT().RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	relay.EXPECT().SubmitBlock(gomock.Any(), gomock.Any()).AnyTimes()

	randReqs := make([]*http.Request, 0)
	ws := make([]*httptest.ResponseRecorder, 0)

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := reqs[rand.Intn(len(reqs))]
		randReq := httptest.NewRequest(req.Method, req.URL.Path, nil)
		randReqs = append(randReqs, randReq)
		ws = append(ws, w)
	}

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func(i int) {
			mux.ServeHTTP(ws[i], randReqs[i])
			wg.Done()
		}(i)
	}
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}
