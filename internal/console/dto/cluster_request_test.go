package dto

import (
	"encoding/json"
	"testing"
)

func TestClusterCreateInputValidatesFinalDuplicatePullPolicy(t *testing.T) {
	data := []byte(`{"name":"test","installParams":{"namespace":"kubesentry-system","webhookImage":{"repository":"webhook","tag":"latest","pullPolicy":"bad"},"operatorImage":{"repository":"operator","tag":"latest","pullPolicy":"IfNotPresent"},"highAvailability":false,"gatewayAddress":"gateway"},"installParams":{"namespace":"kubesentry-system","webhookImage":{"repository":"webhook","tag":"latest","pullPolicy":"Always"},"operatorImage":{"repository":"operator","tag":"latest","pullPolicy":"Never"},"highAvailability":false,"gatewayAddress":"gateway"}}`)
	var input ClusterCreateInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestClusterTokenInputRejectsInvalidPullPolicy(t *testing.T) {
	data := []byte(`{"installParams":{"namespace":"kubesentry-system","webhookImage":{"repository":"webhook","tag":"latest","pullPolicy":"bad"},"operatorImage":{"repository":"operator","tag":"latest","pullPolicy":"IfNotPresent"},"highAvailability":false,"gatewayAddress":"gateway"}}`)
	var input ClusterTokenInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() succeeded, want invalid pullPolicy error")
	}
}
