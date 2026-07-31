package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

const platformSampleEnvelopeTarget = "platform-observability"

func (s *Server) handleMemberComponentSamples(
	ctx context.Context,
	clusterName, operatorInstance, operatorRole string,
	batch *gatewaypb.ComponentSampleBatch,
) error {
	if s.repos.Platform == nil || batch == nil {
		return nil
	}
	cluster, err := s.repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("member cluster not found")
		}
		return err
	}
	for _, wire := range batch.Samples {
		sample, err := platformobs.FromProto(wire)
		if err != nil {
			return err
		}
		if sample.Component != platformobs.ComponentOperator &&
			sample.Component != platformobs.ComponentWebhook {
			return fmt.Errorf("member cannot report component %q", sample.Component)
		}
		sample.ClusterID = cluster.ID
		sample.ClusterName = cluster.Name
		if sample.Component == platformobs.ComponentOperator {
			sample.Instance = operatorInstance
			sample.Role = platformobs.Role(operatorRole)
		} else {
			sample.Role = platformobs.RoleNone
		}
		if _, err := s.repos.Platform.UpsertSample(ctx, sample, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleGatewayComponentSamples(ctx context.Context, batch *gatewaypb.ComponentSampleBatch) error {
	if s.repos.Platform == nil || batch == nil {
		return nil
	}
	for _, wire := range batch.Samples {
		sample, err := platformobs.FromProto(wire)
		if err != nil {
			return err
		}
		if sample.Component != platformobs.ComponentConsoleAPI &&
			sample.Component != platformobs.ComponentConsoleWeb {
			return fmt.Errorf("connect stream cannot report component %q", sample.Component)
		}
		sample.ClusterID = 0
		sample.ClusterName = ""
		sample.Role = platformobs.RoleNone
		if _, err := s.repos.Platform.UpsertSample(ctx, sample, time.Now()); err != nil {
			return err
		}
	}
	return nil
}
