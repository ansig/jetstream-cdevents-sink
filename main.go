package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ansig/jetstream-cdevents-sink/internal/adapter"
	"github.com/ansig/jetstream-cdevents-sink/internal/invalidmsg"
	"github.com/ansig/jetstream-cdevents-sink/internal/sink"
	"github.com/ansig/jetstream-cdevents-sink/internal/translator"
	"github.com/ansig/jetstream-cdevents-sink/internal/transport"
	"github.com/ansig/jetstream-cdevents-sink/internal/webhook"

	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var logger *slog.Logger

var translators = map[string]translator.Webhook{
	"gitea.push":         &translator.GiteaPush{},
	"gitea.pull_request": &translator.GiteaPullRequest{},
	"gitea.create":       &translator.GiteaCreate{},
	"gitea.delete":       &translator.GiteaDelete{},
}

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
var logLevel = flag.String("log-level", "info", "The event level to output.")

type envConfig struct {
	NATSUrl             string `envconfig:"NATS_URL" default:"http://localhost:4222" required:"true"`
	WebhookStreamName   string `envconfig:"WEBHOOK_STREAM_NAME" default:"webhook-adapter-queue" required:"true"`
	WebhookSubjectBase  string `envconfig:"WEBHOOK_SUBJECT_BASE" default:"webhooks" required:"true"`
	WebhookConsumerName string `envconfig:"WEBHOOK_CONSUMER_NAME" default:"webhook-adapter" required:"true"`
	InvMsgStreamName    string `envconfig:"INVALID_MESSAGES_STREAM_NAME" default:"invalid-messages-channel" required:"true"`
	InvMsgSubjectBase   string `envconfig:"INVALID_MESSAGES_SUBJECT_BASE" default:"invalid" required:"true"`
	InvMsgStreamMaxAge  string `envconfig:"INVALID_MESSAGES_STREAM_MAX_AGE" default:"48h" required:"true"`
	EventStreamName     string `envconfig:"EVENT_STREAM_NAME" default:"cdevents" required:"true"`
	EventSubjectBase    string `envconfig:"EVENT_SUBJECT_BASE" default:"dev.cdevents" required:"true"`
	EventStreamMaxAge   string `envconfig:"EVENT_STREAM_MAX_AGE" default:"8808h" required:"true"`
}

func MustCreateStream(ctx context.Context, jetstream natsjs.JetStream, config natsjs.StreamConfig) natsjs.Stream {

	var stream natsjs.Stream

	logger.Debug(fmt.Sprintf("Setting up Jetstream: %s", config.Name), "config", config)

	stream, err := jetstream.CreateStream(ctx, config)
	if err == natsjs.ErrStreamNameAlreadyInUse {
		logger.Warn(fmt.Sprintf("Updating existing stream: %s", config.Name))
		stream, err = jetstream.UpdateStream(ctx, config)
		if err != nil {
			logger.Error("Failed to update existing stream", "error", err.Error())
			os.Exit(1)
		}
	} else if err != nil {
		logger.Error("Error when creating stream", "error", err.Error())
		os.Exit(1)
	}

	return stream
}

func main() {

	flag.Parse()

	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		fmt.Printf("Error when processing envvar configuration: %v\n", err)
		os.Exit(1)
	}

	var programLevel = new(slog.LevelVar)
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel}))

	switch strings.ToLower(*logLevel) {
	case "debug":
		programLevel.Set(slog.LevelDebug)
	case "info":
		programLevel.Set(slog.LevelInfo)
	case "error":
		programLevel.Set(slog.LevelError)
	case "warn":
		programLevel.Set(slog.LevelWarn)
	default:
		logger.Warn(fmt.Sprintf("Unknown log level: %s (using default: %s)", *logLevel, programLevel.Level()))
	}

	logger.Info(fmt.Sprintf("Connecting to Nats on %s...", env.NATSUrl))

	nc, err := nats.Connect(env.NATSUrl)
	if err != nil {
		logger.Error("Failed to connect to nats", "error", err.Error())
		os.Exit(1)
	}

	defer nc.Close()

	jetstream, err := natsjs.New(nc)
	if err != nil {
		logger.Error("Failed to create JetStream instance", "error", err.Error())
		os.Exit(1)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()

	eventStreamMaxAge, err := time.ParseDuration(env.EventStreamMaxAge)
	if err != nil {
		logger.Error("Failed to parse stream age", "error", err)
		os.Exit(1)
	}

	MustCreateStream(startupCtx, jetstream, natsjs.StreamConfig{
		Name:        env.EventStreamName,
		Subjects:    []string{fmt.Sprintf("%s.>", env.EventSubjectBase)},
		Description: "Output stream for Webhook",
		Retention:   natsjs.LimitsPolicy,
		MaxAge:      eventStreamMaxAge,
		Discard:     natsjs.DiscardOld,
	})

	invalidMsgStreamMaxAge, err := time.ParseDuration(env.InvMsgStreamMaxAge)
	if err != nil {
		logger.Error("Failed to parse stream age", "error", err)
		os.Exit(1)
	}

	MustCreateStream(startupCtx, jetstream, natsjs.StreamConfig{
		Name:        env.InvMsgStreamName,
		Subjects:    []string{fmt.Sprintf("%s.>", env.InvMsgSubjectBase)},
		Description: "Invalid message channel",
		Retention:   natsjs.LimitsPolicy,
		MaxAge:      invalidMsgStreamMaxAge,
		Discard:     natsjs.DiscardOld,
	})

	webhookStream := MustCreateStream(startupCtx, jetstream, natsjs.StreamConfig{
		Name:        env.WebhookStreamName,
		Subjects:    []string{fmt.Sprintf("%s.>", env.WebhookSubjectBase)},
		Description: "Work queue stream for incoming webhooks",
		Retention:   natsjs.WorkQueuePolicy,
	})

	consumer, err := webhookStream.CreateOrUpdateConsumer(startupCtx, natsjs.ConsumerConfig{
		Durable:   env.WebhookConsumerName,
		AckPolicy: natsjs.AckExplicitPolicy,
	})

	if err != nil {
		logger.Error("Could not create consumer", "error", err.Error())
		os.Exit(1)
	}

	done := make(chan interface{})

	messages := make(chan natsjs.Msg)
	consContext, _ := consumer.Consume(func(msg natsjs.Msg) {
		messages <- msg
	})

	var wg sync.WaitGroup

	invalidMessageHandler := invalidmsg.NewJetStreamInvalidMsgHandler(
		logger,
		jetstream,
		env.InvMsgSubjectBase,
	)

	cdEventsAdapter := adapter.New(logger, nc, translators, invalidMessageHandler)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer consContext.Stop()
		for {
			select {
			case msg := <-messages:
				if err := cdEventsAdapter.Process(msg); err != nil {
					logger.Error("Failed to process message", "error", err.Error())
				}
			case <-done:
				logger.Info("Stopped processing messages")
				return
			}
		}
	}()

	logger.Info("JetStream consumer ready and listening...")

	logger.Info("Starting server...")

	webhook := webhook.New(logger)
	sink := sink.New(logger)

	cloudEventPublisher := transport.NewCloudEventJetStreamPublisher(nc)

	mux := http.NewServeMux()
	mux.Handle("/webhook", webhook.Handler(jetstream, env.WebhookSubjectBase))
	mux.Handle("/sink", sink.Handler(cloudEventPublisher))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if nc.IsConnected() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("READY"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	mux.Handle("/metrics", promhttp.Handler())

	srv := http.Server{
		Addr:         *addr,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  90 * time.Second,
		Handler:      mux,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
		s := <-interrupt

		logger.Info("Received interrupt signal", "signal", s)

		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second*10)
		defer cancelShutdown()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error when shutting down server", "error", err.Error())
			os.Exit(1)
		}
	}()

	logger.Info(fmt.Sprintf("Server listening on %s...", *addr))

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("Error from listen and server", "error", err.Error())
		os.Exit(1)
	}

	close(done)

	logger.Info("Gracefully shutting down...")

	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		logger.Info("All done, exit program")
	case <-time.After(time.Second * 30):
		logger.Error("Timeout waiting for all goroutines to finish")
		os.Exit(1)
	}
}
