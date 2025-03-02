package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ansig/jetstream-cdevents-sink/internal/invalidmsg"
	"github.com/ansig/jetstream-cdevents-sink/internal/translator"
	"github.com/ansig/jetstream-cdevents-sink/internal/transport"

	"github.com/nats-io/nats.go"
)

var (
	ErrInvalidSubject    error = errors.New("Message subject is invalid")
	ErrNoTranslator      error = errors.New("No translator found")
	ErrTranslationFailed error = errors.New("Could not translate event")
	ErrPublishFailed     error = errors.New("Failed to publish event")
)

type CDEvents struct {
	logger        *slog.Logger
	publisher     transport.CloudEventPublisher
	invMsgHandler invalidmsg.Handler
	translators   map[string]translator.Webhook
}

func New(logger *slog.Logger, nc *nats.Conn, translators map[string]translator.Webhook, invMsgHandler invalidmsg.Handler) *CDEvents {
	return &CDEvents{
		logger:        logger,
		publisher:     transport.NewCloudEventJetStreamPublisher(nc),
		translators:   translators,
		invMsgHandler: invMsgHandler,
	}
}

func (c *CDEvents) Process(msg transport.JetstreamMsg) error {

	defer msg.Ack()

	metadata, err := msg.Metadata()
	if err != nil {
		return err
	}

	c.logger.Debug("Processing incoming webhook message",
		"subject", msg.Subject(),
		"stream_seq", metadata.Sequence.Stream,
		"num_delivered", metadata.NumDelivered,
		"stream", metadata.Stream,
		"consumer", metadata.Consumer)

	var v map[string]interface{}
	if err := json.Unmarshal(msg.Data(), &v); err != nil {
		return err
	}

	subjectParts := strings.Split(msg.Subject(), ".")
	if len(subjectParts) < 2 {
		c.logger.Error(fmt.Sprintf("Unable to determine type of message as subject has to few parts: %s", msg.Subject()))
		if err := c.invMsgHandler.Receive(msg, ErrInvalidSubject); err != nil {
			return err
		}
		return nil
	}

	eventSubject := strings.Join(subjectParts[1:], ".")
	translator, exists := c.translators[eventSubject]
	if !exists {
		c.logger.Error(fmt.Sprintf("No translator found for subject: %s", eventSubject))
		if err := c.invMsgHandler.Receive(msg, ErrNoTranslator); err != nil {
			return err
		}
		return nil
	}

	cdEvent, err := translator.Translate(msg.Data())
	if err != nil {
		c.logger.Error("Failed to translate event", "error", err)
		if err := c.invMsgHandler.Receive(msg, ErrTranslationFailed); err != nil {
			return err
		}
		return nil
	}

	c.logger.Debug("Translated incoming webhook message into CDEvent",
		"type", cdEvent.GetType(),
		"subject", msg.Subject(),
		"stream_seq", metadata.Sequence.Stream,
		"num_delivered", metadata.NumDelivered,
		"stream", metadata.Stream,
		"consumer", metadata.Consumer)

	if err := c.publisher.Publish(cdEvent); err != nil {
		c.logger.Error("Failed to publish CDEvent", "error", err)
		if err := c.invMsgHandler.Receive(msg, ErrPublishFailed); err != nil {
			return err
		}
		return nil
	}

	return nil
}
