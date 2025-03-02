package invalidmsg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ansig/jetstream-cdevents-sink/internal/transport"
)

type Holder struct {
	Subject      string      `json:"subject"`
	Timestamp    time.Time   `json:"timestamp"`
	StreamSeq    uint64      `json:"stream_seq"`
	NumDelivered uint64      `json:"num_delivered"`
	Error        string      `json:"error"`
	Content      interface{} `json:"content"`
}

type Handler interface {
	Receive(invalidMsg transport.JetstreamMsg, originalErr error) error
}

type jetStreamInvalidMsgHandler struct {
	logger              *slog.Logger
	publisher           transport.JetstreamPublisher
	outgoingSubjectBase string
}

func NewJetStreamInvalidMsgHandler(logger *slog.Logger, publisher transport.JetstreamPublisher, outgoingSubjectBase string) Handler {
	return &jetStreamInvalidMsgHandler{
		logger:              logger,
		publisher:           publisher,
		outgoingSubjectBase: outgoingSubjectBase,
	}
}

func (i *jetStreamInvalidMsgHandler) Receive(invalidMsg transport.JetstreamMsg, originalErr error) error {

	invalidMsgMetadata, err := invalidMsg.Metadata()
	if err != nil {
		return err
	}

	i.logger.Debug("Handling invalid message",
		"subject", invalidMsg.Subject(),
		"stream_seq", invalidMsgMetadata.Sequence.Stream,
		"num_delivered", invalidMsgMetadata.NumDelivered,
		"stream", invalidMsgMetadata.Stream)

	var invalidMsgContent interface{}
	if err := json.Unmarshal(invalidMsg.Data(), &invalidMsgContent); err != nil {
		i.logger.Error("Invalid message handler failed to unmarchal message data", "error", err)
		return err
	}

	outgoingMsgData, err := json.Marshal(Holder{
		Subject:      invalidMsg.Subject(),
		Content:      invalidMsgContent,
		Timestamp:    invalidMsgMetadata.Timestamp,
		StreamSeq:    invalidMsgMetadata.Sequence.Stream,
		NumDelivered: invalidMsgMetadata.NumDelivered,
		Error:        originalErr.Error(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := i.publisher.Publish(ctx, fmt.Sprintf("%s.%s", i.outgoingSubjectBase, invalidMsg.Subject()), outgoingMsgData); err != nil {
		i.logger.Error("Invalid message handler failed to publish message", "error", err)
		return err
	}

	return nil
}
