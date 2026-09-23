package observability

import (
	"context"
	"time"
)

const DefaultInterval = time.Minute

func Run(ctx context.Context, interval time.Duration, collect func() Sample, submit func(Sample) error) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := submit(collect()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
