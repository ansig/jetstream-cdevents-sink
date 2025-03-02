package sink

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ansig/jetstream-cdevents-sink/internal/mocks"
	cdeventsv04 "github.com/cdevents/sdk-go/pkg/api/v04"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSinkHandler(t *testing.T) {

	mockPublisher := &mocks.CloudEventPublisher{}
	mockPublisher.On("Publish", mock.Anything).Return(nil)

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	changeMergedEvent, err := cdeventsv04.NewChangeMergedEvent()
	require.NoError(t, err, "unable to create CDEvent for tests")

	data, err := json.Marshal(changeMergedEvent)
	require.NoError(t, err, "failed to unmarchal CDEvent for testing")

	body := strings.NewReader(string(data))

	req := httptest.NewRequest("POST", "/", body)

	requestHeaders := map[string][]string{
		"Content-Type": {"application/json"},
	}

	for k, v := range requestHeaders {
		req.Header.Add(k, strings.Join(v, ","))
	}

	rec := httptest.NewRecorder()

	sink := New(testLogger)
	sink.Handler(mockPublisher).ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	responseBody, _ := io.ReadAll(res.Body)
	assert.Equal(t, "OK", strings.TrimSpace(string(responseBody)), "Response body should be \"OK\"")

	mockPublisher.AssertCalled(t, "Publish", mock.Anything)
}
