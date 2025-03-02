package invalidmsg

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ansig/jetstream-cdevents-sink/internal/mocks"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestJetStreamInvalidMsgHandler(t *testing.T) {

	mockPublisher := &mocks.JetstreamPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	outgoingSubjectBase := "invalid"

	handler := NewJetStreamInvalidMsgHandler(logger, mockPublisher, outgoingSubjectBase)

	invalidMsgDeliveryTime, err := time.Parse(time.RFC3339, "2025-02-23T09:30:00+01:00")
	require.NoError(t, err, "Failed to create timestamp for test")

	invalidMsgContent := struct {
		foo string
		baz string
	}{
		foo: "bar",
		baz: "qux",
	}

	invalidMsgContentBytes, err := json.Marshal(invalidMsgContent)
	require.NoError(t, err, "Failed to marchal orignal content for test")

	invalidMsgSubject := "webhooks.foo"
	invalidMsg := mocks.NewJetstreamMsg(invalidMsgSubject, invalidMsgContentBytes)
	invalidMsg.StreamSeq = 123
	invalidMsg.NumDelivered = 1
	invalidMsg.Timestamp = invalidMsgDeliveryTime

	mockPublisher.On("Publish", mock.Anything, mock.Anything).Return(&jetstream.PubAck{Stream: "mockStream"}, nil)

	err = handler.Receive(invalidMsg, fmt.Errorf("Could not deliver CD Event"))
	require.NoError(t, err, "should not return an error")

	expectedOutgoingMsgData, err := json.Marshal(Holder{
		Subject:      invalidMsgSubject,
		Content:      invalidMsgContent,
		StreamSeq:    123,
		NumDelivered: 1,
		Timestamp:    invalidMsgDeliveryTime,
		Error:        "Could not deliver CD Event",
	})
	require.NoError(t, err, "Failed to create expected message data")

	expectedOutgoingSubject := fmt.Sprintf("%s.%s", outgoingSubjectBase, invalidMsgSubject)

	mockPublisher.AssertCalled(t, "Publish", expectedOutgoingSubject, expectedOutgoingMsgData)
}
