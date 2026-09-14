package dto

type LabelSelectorRequirementView struct {
	Key      string   `json:"key" binding:"required"`
	Operator string   `json:"operator" binding:"required" enums:"In,NotIn,Exists,DoesNotExist"`
	Values   []string `json:"values,omitempty"`
}

type LabelSelectorView struct {
	MatchLabels      map[string]string              `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirementView `json:"matchExpressions,omitempty"`
}
