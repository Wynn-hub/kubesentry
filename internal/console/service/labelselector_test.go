package service

import (
	"reflect"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestLabelSelectorRoundTrip(t *testing.T) {
	want := &dto.LabelSelectorView{
		MatchLabels: map[string]string{"environment": "production"},
		MatchExpressions: []dto.LabelSelectorRequirementView{{
			Key: "region", Operator: "In", Values: []string{"east", "west"},
		}},
	}
	if got := labelSelectorView(labelSelectorModel(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	if labelSelectorModel(nil) != nil || labelSelectorView(nil) != nil {
		t.Fatal("nil selector must remain nil")
	}
}
