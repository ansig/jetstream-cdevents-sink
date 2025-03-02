package sink

import (
	"io"
	"log/slog"
	"mime"
	"net/http"

	cdeventsv04 "github.com/cdevents/sdk-go/pkg/api/v04"

	"github.com/ansig/jetstream-cdevents-sink/internal/transport"
)

type sink struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *sink {
	return &sink{
		logger: logger,
	}
}

func (s *sink) Handler(cePublisher transport.CloudEventPublisher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			http.Error(w, "Method not supported", http.StatusNotImplemented)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			http.Error(w, "Content-Type header not set", http.StatusBadRequest)
			return
		}

		mt, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			http.Error(w, "Malformed Content-Type header", http.StatusBadRequest)
			return
		}

		if mt != "application/json" {
			http.Error(w, "Content-Type header must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		data, err := io.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("Failure when reading request body", "error", err.Error())
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if len(data) == 0 {
			http.Error(w, "Received empty body", http.StatusBadRequest)
			return
		}

		cdevent, err := cdeventsv04.NewFromJsonBytes(data)
		if err != nil {
			s.logger.Error("Sink failed to create CDEvent from payload", "error", err)
			http.Error(w, "Payload is not a valid CDEvent", http.StatusBadRequest)
			return
		}

		if err := cePublisher.Publish(cdevent); err != nil {
			s.logger.Error("Sink failed to publish CDEvent", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
}
