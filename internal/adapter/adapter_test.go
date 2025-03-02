package adapter

import (
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/ansig/jetstream-cdevents-sink/internal/mocks"
	"github.com/ansig/jetstream-cdevents-sink/internal/translator"
	"github.com/ansig/jetstream-cdevents-sink/internal/transport"
	cdevents "github.com/cdevents/sdk-go/pkg/api"
	cdeventsv04 "github.com/cdevents/sdk-go/pkg/api/v04"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProcess(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	changeMergedEvent, err := cdeventsv04.NewChangeMergedEvent()
	require.NoError(t, err, "unable to create CDEvent for tests")

	validMsgData := []byte("{\"foo\": \"bar\"}")

	webhookTestEventMsg := mocks.NewJetstreamMsg("webhook.test.event", validMsgData)
	webhookTestUnknownMsg := mocks.NewJetstreamMsg("webhook.unknown", validMsgData)
	invalidSubjectMsg := mocks.NewJetstreamMsg("invalid", validMsgData)

	for _, tc := range []struct {
		title                     string
		incomingMsg               transport.JetstreamMsg
		translatorSubject         string
		translatedEvent           cdevents.CDEvent
		translatorError           error
		publisherError            error
		invalidMsgHandlerError    error
		expectedError             error
		expectedDataTranslated    []byte
		expectedEventPublished    cdevents.CDEvent
		expectedInvMsgHandlerArgs []interface{}
		expectedMsgSentToHandler  transport.JetstreamMsg
		expectedErrSentToHandler  error
	}{
		{
			title:                  "translates message data and publishes translated event",
			incomingMsg:            webhookTestEventMsg,
			translatorSubject:      "test.event",
			translatedEvent:        changeMergedEvent,
			expectedDataTranslated: webhookTestEventMsg.Data(),
			expectedEventPublished: changeMergedEvent,
		},
		{
			title:                     "send to invalid msg handler when no translator matching subject",
			incomingMsg:               webhookTestUnknownMsg,
			translatorSubject:         "test.somethingelse", // different from that of webhoostTestUnkownMsg
			expectedInvMsgHandlerArgs: []interface{}{webhookTestUnknownMsg, ErrNoTranslator},
		},
		{
			title:                     "send to invalid msg handler on less than 2 subject parts",
			incomingMsg:               invalidSubjectMsg,
			translatorSubject:         "test.event",
			expectedInvMsgHandlerArgs: []interface{}{invalidSubjectMsg, ErrInvalidSubject},
		},
		{
			title:                     "send to invalid msg handler when translator returns error",
			incomingMsg:               webhookTestEventMsg,
			translatorSubject:         "test.event",
			translatorError:           fmt.Errorf("something went wrong in translating the event"),
			expectedInvMsgHandlerArgs: []interface{}{webhookTestEventMsg, ErrTranslationFailed},
		},
		{
			title:                     "send to invalid msg handler when publish returns error",
			incomingMsg:               webhookTestEventMsg,
			translatorSubject:         "test.event",
			translatedEvent:           changeMergedEvent,
			publisherError:            fmt.Errorf("something went wrong when publishing the event"),
			expectedInvMsgHandlerArgs: []interface{}{webhookTestEventMsg, ErrPublishFailed},
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			mockPublisher := &mocks.CloudEventPublisher{}
			mockPublisher.On("Publish", mock.Anything).Return(tc.publisherError)

			mockTranslator := &mocks.MockWebhookTranslator{}
			mockTranslator.On("Translate", mock.Anything).Return(tc.translatedEvent, tc.translatorError)

			mockInvMsgHandler := &mocks.MockInvalidMessageHandler{}
			mockInvMsgHandler.On("Receive", mock.Anything, mock.Anything).Return(tc.invalidMsgHandlerError)

			adapter := &CDEvents{
				logger:        logger,
				publisher:     mockPublisher,
				invMsgHandler: mockInvMsgHandler,
				translators:   map[string]translator.Webhook{tc.translatorSubject: mockTranslator},
			}

			err = adapter.Process(tc.incomingMsg)

			if tc.expectedError != nil {
				assert.ErrorIs(t, err, tc.expectedError, "error should be returned")
			} else {
				require.NoError(t, err, "no error should be returned")
			}

			if tc.expectedDataTranslated != nil {
				mockTranslator.AssertCalled(t, "Translate", tc.expectedDataTranslated)
			}

			if tc.expectedEventPublished != nil {
				mockPublisher.AssertCalled(t, "Publish", tc.expectedEventPublished)
			}

			if tc.expectedInvMsgHandlerArgs != nil {
				mockInvMsgHandler.AssertCalled(t, "Receive", tc.expectedInvMsgHandlerArgs...)
			}
		})
	}
}
