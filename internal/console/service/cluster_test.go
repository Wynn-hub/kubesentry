package service

import (
	"strings"
	"testing"
)

func TestValidateInstallParamsRejectsShellMetacharacters(t *testing.T) {
	params := ClusterInstallParams{
		Namespace:      "kubesentry-system",
		WebhookImage:   ClusterImage{Repository: "example/webhook", Tag: "local", PullPolicy: "IfNotPresent"},
		OperatorImage:  ClusterImage{Repository: "example/operator", Tag: "local", PullPolicy: "IfNotPresent"},
		GatewayAddress: "gateway:30443",
	}
	params.GatewayAddress = "gateway:30443$HOME"
	if err := validateInstallParams(&params); err == nil {
		t.Fatal("expected invalid gateway address")
	}
}

func TestBuildInstallCommandIncludesHAAndRegistration(t *testing.T) {
	params := ClusterInstallParams{
		Namespace:        "kubesentry-system",
		WebhookImage:     ClusterImage{Repository: "example/webhook", Tag: "local", PullPolicy: "IfNotPresent"},
		OperatorImage:    ClusterImage{Repository: "example/operator", Tag: "local", PullPolicy: "IfNotPresent"},
		HighAvailability: true,
		GatewayAddress:   "gateway:30443",
	}
	command := buildInstallCommand("local", "token", &params)
	if strings.Contains(command, "\n+") {
		t.Fatalf("command contains patch-marker pluses: %s", command)
	}
	for _, want := range []string{
		"helm upgrade --install kubesentry charts/kubesentry",
		"--namespace kubesentry-system --create-namespace",
		"--set webhook.image.repository=\"example/webhook\"",
		"--set operator.image.tag=\"local\"",
		"--set webhook.highAvailability.enabled=true",
		"--set operator.highAvailability.enabled=true",
		"--set gateway.clusterName=\"local\"",
		"--set gateway.registrationToken=\"token\"",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
}
