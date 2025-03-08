package mocks

import (
	"context"
	"time"

	"github.com/ansig/jetstream-cdevents-sink/internal/transport"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/mock"

	cdevents "github.com/cdevents/sdk-go/pkg/api"
)

type CloudEventPublisher struct {
	mock.Mock
}

func (m *CloudEventPublisher) Publish(cdEvent cdevents.CDEvent) error {
	args := m.Called(cdEvent)
	return args.Error(0)
}

type JetstreamPublisher struct {
	mock.Mock
}

func (m *JetstreamPublisher) Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	args := m.Called(subject, data)
	if args.Get(0) == nil {
		return nil, args.Error(1) // Because otherwise we will panic on the type conversion below when first argument is nil
	}
	return args.Get(0).(*jetstream.PubAck), args.Error(1)
}

type JetstreamMsg struct {
	mock.Mock
	subject      string
	data         []byte
	Acked        bool
	ConsumerSeq  uint64
	StreamSeq    uint64
	NumDelivered uint64
	Timestamp    time.Time
}

func (m *JetstreamMsg) Subject() string { return m.subject }
func (m *JetstreamMsg) Data() []byte    { return m.data }
func (m *JetstreamMsg) Ack() error {
	m.Acked = true
	return nil
}
func (m *JetstreamMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{
		Sequence: jetstream.SequencePair{
			Stream:   m.StreamSeq,
			Consumer: m.ConsumerSeq,
		},
		NumDelivered: m.NumDelivered,
		Timestamp:    m.Timestamp,
	}, nil
}

func NewJetstreamMsg(subject string, data []byte) *JetstreamMsg {
	return &JetstreamMsg{
		subject: subject,
		data:    data,
	}
}

type WebhookTranslator struct {
	mock.Mock
}

func (m *WebhookTranslator) Translate(data []byte) (cdevents.CDEvent, error) {
	args := m.Called(data)
	if args.Get(0) == nil {
		return nil, args.Error(1) // Because otherwise we will panic on the type conversion below when first argument is nil
	}
	return args.Get(0).(cdevents.CDEvent), args.Error(1)
}

type InvalidMessageHandler struct {
	mock.Mock
}

func (m *InvalidMessageHandler) Receive(invalidMsg transport.JetstreamMsg, originalErr error) error {
	args := m.Called(invalidMsg, originalErr)
	return args.Error(0)
}
