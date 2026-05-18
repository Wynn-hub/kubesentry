package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "kubesentry.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
	SchemeBuilder.Register(&PolicyVersion{}, &PolicyVersionList{})
	SchemeBuilder.Register(&PolicyGroup{}, &PolicyGroupList{})
	SchemeBuilder.Register(&PolicyException{}, &PolicyExceptionList{})
}
