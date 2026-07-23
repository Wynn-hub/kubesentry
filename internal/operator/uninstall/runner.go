// Package uninstall removes only the KubeSentry release resources owned by a
// member cluster. It intentionally never deletes a Namespace.
package uninstall

import (
	"context"
	"fmt"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// Runner uses a fixed allowlist. Namespace is deliberately not represented by
// a delete target, preventing a control-plane installation in the same
// namespace from being removed by a member-cluster uninstall request.
type Runner struct {
	Client    client.Client
	Namespace string
}

func objectMeta(name string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: name} }
func namespacedMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: namespace}
}

// Run removes all release resources except the operator process and its
// credentials. Keeping that small communication path alive lets the member
// report completion to the hub before it removes itself in Finalize.
func (r Runner) Run(ctx context.Context) error {
	if r.Client == nil || r.Namespace == "" {
		return fmt.Errorf("client and namespace are required")
	}
	var errs []string
	delete := func(obj client.Object) {
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err.Error())
		}
	}

	// Disable admission first, then remove KubeSentry CR instances before their
	// definitions. CR kinds are exclusive to this release, unlike Namespace.
	delete(&admissionv1.ValidatingWebhookConfiguration{ObjectMeta: objectMeta("kubesentry")})
	for _, name := range []string{"kubesentry-policyexception-immutability"} {
		delete(&admissionv1.ValidatingAdmissionPolicy{ObjectMeta: objectMeta(name)})
	}
	for _, name := range []string{"kubesentry-policyexception-immutability-binding"} {
		delete(&admissionv1.ValidatingAdmissionPolicyBinding{ObjectMeta: objectMeta(name)})
	}
	r.deleteCRs(ctx, &errs)

	for _, name := range []string{"kubesentry-webhook"} {
		delete(&appsv1.Deployment{ObjectMeta: namespacedMeta(name, r.Namespace)})
	}
	delete(&batchv1.Job{ObjectMeta: namespacedMeta("kubesentry-tls-setup", r.Namespace)})
	delete(&corev1.Service{ObjectMeta: namespacedMeta("kubesentry-webhook", r.Namespace)})
	for _, name := range []string{"kubesentry-webhook-tls", "kubesentry-hub-register", "kubesentry-hub-ca"} {
		delete(&corev1.Secret{ObjectMeta: namespacedMeta(name, r.Namespace)})
	}
	for _, name := range []string{"kubesentry-webhook", "kubesentry-tls-setup"} {
		delete(&corev1.ServiceAccount{ObjectMeta: namespacedMeta(name, r.Namespace)})
	}
	for _, name := range []string{"kubesentry-tls-setup"} {
		delete(&rbacv1.Role{ObjectMeta: namespacedMeta(name, r.Namespace)})
		delete(&rbacv1.RoleBinding{ObjectMeta: namespacedMeta(name, r.Namespace)})
	}
	for _, name := range []string{"kubesentry-webhook", "kubesentry-tls-setup"} {
		delete(&rbacv1.ClusterRole{ObjectMeta: objectMeta(name)})
		delete(&rbacv1.ClusterRoleBinding{ObjectMeta: objectMeta(name)})
	}
	for _, name := range []string{"policies.kubesentry.io", "policygroups.kubesentry.io", "policyexceptions.kubesentry.io", "policyversions.kubesentry.io"} {
		delete(&apiextensionsv1.CustomResourceDefinition{ObjectMeta: objectMeta(name)})
	}
	if len(errs) > 0 {
		return fmt.Errorf("uninstall KubeSentry resources: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Finalize removes the last operator-side resources after completion has been
// acknowledged by the hub. It intentionally never targets the Namespace.
func (r Runner) Finalize(ctx context.Context) error {
	if r.Client == nil || r.Namespace == "" {
		return fmt.Errorf("client and namespace are required")
	}
	var errs []string
	delete := func(obj client.Object) {
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err.Error())
		}
	}
	delete(&corev1.Secret{ObjectMeta: namespacedMeta("kubesentry-hub-auth", r.Namespace)})
	delete(&appsv1.Deployment{ObjectMeta: namespacedMeta("kubesentry-operator", r.Namespace)})
	delete(&corev1.ServiceAccount{ObjectMeta: namespacedMeta("kubesentry-operator", r.Namespace)})
	delete(&rbacv1.Role{ObjectMeta: namespacedMeta("kubesentry-operator", r.Namespace)})
	delete(&rbacv1.RoleBinding{ObjectMeta: namespacedMeta("kubesentry-operator", r.Namespace)})
	delete(&rbacv1.ClusterRole{ObjectMeta: objectMeta("kubesentry-operator")})
	delete(&rbacv1.ClusterRoleBinding{ObjectMeta: objectMeta("kubesentry-operator")})
	if len(errs) > 0 {
		return fmt.Errorf("finalize KubeSentry uninstall: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (r Runner) deleteCRs(ctx context.Context, errs *[]string) {
	deleteAll := func(list client.ObjectList) {
		if err := r.Client.List(ctx, list); err != nil {
			*errs = append(*errs, err.Error())
			return
		}
		// Each custom kind belongs exclusively to KubeSentry. We deliberately do
		// not list/delete arbitrary core or workload objects.
		switch v := list.(type) {
		case *v1alpha1.PolicyList:
			for i := range v.Items {
				if err := r.Client.Delete(ctx, &v.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					*errs = append(*errs, err.Error())
				}
			}
		case *v1alpha1.PolicyGroupList:
			for i := range v.Items {
				if err := r.Client.Delete(ctx, &v.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					*errs = append(*errs, err.Error())
				}
			}
		case *v1alpha1.PolicyExceptionList:
			for i := range v.Items {
				if err := r.Client.Delete(ctx, &v.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					*errs = append(*errs, err.Error())
				}
			}
		case *v1alpha1.PolicyVersionList:
			for i := range v.Items {
				if err := r.Client.Delete(ctx, &v.Items[i]); err != nil && !apierrors.IsNotFound(err) {
					*errs = append(*errs, err.Error())
				}
			}
		}
	}
	deleteAll(&v1alpha1.PolicyList{})
	deleteAll(&v1alpha1.PolicyGroupList{})
	deleteAll(&v1alpha1.PolicyExceptionList{})
	deleteAll(&v1alpha1.PolicyVersionList{})
}
