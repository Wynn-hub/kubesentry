package console

import (
	"net/http"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

var platformSampleInterval = platformobs.DefaultInterval

func NewPlatformSampleSource(instance, webURL string) func() *gatewaypb.ComponentSampleBatch {
	apiCollector := platformobs.NewCollector(platformobs.Identity{
		Component: platformobs.ComponentConsoleAPI,
		Instance:  instance,
		Role:      func() platformobs.Role { return platformobs.RoleNone },
	})
	startedAt := platformobs.NormalizeProcessStart(time.Now())
	client := &http.Client{Timeout: 2 * time.Second}
	return func() *gatewaypb.ComponentSampleBatch {
		api := apiCollector.Collect()
		web := platformobs.CollectSiblingCgroup(
			platformobs.ComponentConsoleWeb, instance, time.Now().UTC(), startedAt)
		web.HealthReason = "collected_by_console_api"
		if webURL != "" {
			response, err := client.Get(webURL)
			if err != nil {
				web.Health = platformobs.HealthDegraded
				web.HealthReason = "console_web_probe_failed"
			} else {
				_ = response.Body.Close()
				if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
					web.Health = platformobs.HealthDegraded
					web.HealthReason = "console_web_probe_failed"
				}
			}
		}
		return &gatewaypb.ComponentSampleBatch{Samples: []*gatewaypb.ComponentSample{
			platformobs.ToProto(api),
			platformobs.ToProto(web),
		}}
	}
}
