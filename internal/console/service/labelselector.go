package service

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func labelSelectorModel(value *dto.LabelSelectorView) *metav1.LabelSelector {
	if value == nil {
		return nil
	}
	result := &metav1.LabelSelector{MatchLabels: value.MatchLabels, MatchExpressions: make([]metav1.LabelSelectorRequirement, len(value.MatchExpressions))}
	for i := range value.MatchExpressions {
		result.MatchExpressions[i] = metav1.LabelSelectorRequirement{Key: value.MatchExpressions[i].Key, Operator: metav1.LabelSelectorOperator(value.MatchExpressions[i].Operator), Values: value.MatchExpressions[i].Values}
	}
	return result
}

func labelSelectorView(value *metav1.LabelSelector) *dto.LabelSelectorView {
	if value == nil {
		return nil
	}
	result := &dto.LabelSelectorView{MatchLabels: value.MatchLabels, MatchExpressions: make([]dto.LabelSelectorRequirementView, len(value.MatchExpressions))}
	for i := range value.MatchExpressions {
		result.MatchExpressions[i] = dto.LabelSelectorRequirementView{Key: value.MatchExpressions[i].Key, Operator: string(value.MatchExpressions[i].Operator), Values: value.MatchExpressions[i].Values}
	}
	return result
}
