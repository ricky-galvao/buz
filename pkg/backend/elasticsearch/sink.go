// Copyright (c) 2023 Silverton Data, Inc.
// You may use, distribute, and modify this code under the terms of the Apache-2.0 license, a copy of
// which may be found at https://github.com/silverton-io/buz/blob/main/LICENSE

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/rs/zerolog/log"
	"github.com/silverton-io/buz/pkg/backend/backendutils"
	"github.com/silverton-io/buz/pkg/config"
	"github.com/silverton-io/buz/pkg/envelope"
)

type Sink struct {
	metadata backendutils.SinkMetadata
	client   *elasticsearch.Client
	input    chan []envelope.Envelope
	shutdown chan int
}

func (s *Sink) Metadata() backendutils.SinkMetadata {
	return s.metadata
}

func (s *Sink) Initialize(conf config.Sink) error {
	cfg := elasticsearch.Config{
		Addresses: conf.Hosts,
		Username:  conf.User,
		Password:  conf.Password,
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return err
	}
	s.metadata = backendutils.NewSinkMetadataFromConfig(conf)
	s.client = es
	s.input = make(chan []envelope.Envelope, 10000)
	s.shutdown = make(chan int, 1)
	return nil
}

func (s *Sink) StartWorker() error {
	err := backendutils.StartSinkWorker(s.input, s.shutdown, s)
	return err
}

func (s *Sink) Enqueue(envelopes []envelope.Envelope) error {
	log.Debug().Interface("metadata", s.Metadata()).Msg("enqueueing envelopes")
	s.input <- envelopes
	return nil
}

func (s *Sink) Dequeue(ctx context.Context, envelopes []envelope.Envelope, output string) error {
	log.Debug().Interface("metadata", s.Metadata()).Msg("dequeueing envelopes")
	var wg sync.WaitGroup
	for _, envelope := range envelopes {
		eByte, err := json.Marshal(envelope)
		reader := bytes.NewReader(eByte)
		if err != nil {
			log.Error().Err(err).Msg("🔴 could not encode envelope to buffer")
			return err
		} else {
			wg.Add(1)
			envelopeId := envelope.Uuid.String()
			_, err := s.client.Create(output, envelopeId, reader)
			if err != nil {
				log.Error().Interface("envelopeId", envelopeId).Err(err).Msg("🔴 could not publish envelope to elasticsearch")
				return err
			}
			defer wg.Done()
		}
	}
	wg.Wait()
	return nil
}

func (s *Sink) Shutdown() error {
	log.Debug().Interface("metadata", s.metadata).Msg("🟢 shutting down sink")
	s.shutdown <- 1
	return nil
}
