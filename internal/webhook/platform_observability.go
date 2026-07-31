package webhook

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

type PlatformReporter struct {
	endpoint  string
	token     string
	client    *http.Client
	collector *platformobs.Collector
	interval  time.Duration
}

func NewPlatformReporter(eventEndpoint, token, instance string) *PlatformReporter {
	base := strings.TrimSuffix(strings.TrimSuffix(eventEndpoint, "/"), "/platform-samples")
	return &PlatformReporter{
		endpoint: base + "/platform-samples",
		token:    token,
		client:   &http.Client{Timeout: 5 * time.Second},
		collector: platformobs.NewCollector(platformobs.Identity{
			Component: platformobs.ComponentWebhook,
			Instance:  instance,
			Role:      func() platformobs.Role { return platformobs.RoleNone },
		}),
		interval: platformobs.DefaultInterval,
	}
}

func (r *PlatformReporter) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		if err := r.post(ctx, r.collector.Collect()); err != nil {
			slog.Warn("report webhook platform sample", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *PlatformReporter) post(ctx context.Context, sample platformobs.Sample) error {
	body, err := protojson.Marshal(&gatewaypb.ComponentSampleBatch{
		Samples: []*gatewaypb.ComponentSample{platformobs.ToProto(sample)},
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+r.token)
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("platform sample endpoint returned %s", response.Status)
	}
	return nil
}
