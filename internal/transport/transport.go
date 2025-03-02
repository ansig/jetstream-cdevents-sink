package transport

import (
	"context"
	"time"

	cdevents "github.com/cdevents/sdk-go/pkg/api"
	cejsm "github.com/cloudevents/sdk-go/protocol/nats_jetstream/v3"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type JetstreamPublisher interface {
	Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

type JetstreamMsg interface {
	Data() []byte
	Subject() string
	Ack() error
	Metadata() (*jetstream.MsgMetadata, error)
}

type CloudEventPublisher interface {
	Publish(cdEvent cdevents.CDEvent) error
}

type cloudEventJetStreamPublisher struct {
	nc *nats.Conn
}

func NewCloudEventJetStreamPublisher(nc *nats.Conn) *cloudEventJetStreamPublisher {
	return &cloudEventJetStreamPublisher{nc: nc}
}

func (p *cloudEventJetStreamPublisher) Publish(cdEvent cdevents.CDEvent) error {
	cloudEvent, err := cdevents.AsCloudEvent(cdEvent)
	if err != nil {
		return err
	}

	connOpt := cejsm.WithConnection(p.nc)
	sendopt := cejsm.WithSendSubject(cloudEvent.Context.GetType())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	proto, err := cejsm.New(ctx, connOpt, sendopt)
	if err != nil {
		return err
	}

	client, err := cloudevents.NewClient(proto)
	if err != nil {
		return err
	}

	if err := client.Send(ctx, *cloudEvent); err != nil {
		return err
	}

	return nil
}
