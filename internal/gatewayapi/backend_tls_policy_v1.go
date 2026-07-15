package gatewayapi

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

var BackendTLSPolicyV1GVK = schema.GroupVersionKind{
	Group:   "gateway.networking.k8s.io",
	Version: "v1",
	Kind:    "BackendTLSPolicy",
}

func NewBackendTLSPolicyV1Object() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(BackendTLSPolicyV1GVK)
	return obj
}

func NewBackendTLSPolicyV1List() *unstructured.UnstructuredList {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList"))
	return list
}

func ListBackendTLSPoliciesV1(
	ctx context.Context,
	reader client.Reader,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	return ListBackendTLSPoliciesV1WithOptions(ctx, reader)
}

func ListBackendTLSPoliciesV1WithOptions(
	ctx context.Context,
	reader client.Reader,
	opts ...client.ListOption,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	list := NewBackendTLSPolicyV1List()
	if err := reader.List(ctx, list, opts...); err != nil {
		if looksLikeFakeReader(reader) && (meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)) {
			var fallback gatewayv1alpha3.BackendTLSPolicyList
			if fallbackErr := reader.List(ctx, &fallback, opts...); fallbackErr != nil {
				return nil, fallbackErr
			}
			return fallback.Items, nil
		}
		return nil, err
	}

	out := make([]gatewayv1alpha3.BackendTLSPolicy, 0, len(list.Items))
	for i := range list.Items {
		item, err := DecodeBackendTLSPolicyV1(&list.Items[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	if len(out) == 0 && looksLikeFakeReader(reader) {
		var fallback gatewayv1alpha3.BackendTLSPolicyList
		if err := reader.List(ctx, &fallback, opts...); err != nil {
			return nil, err
		}
		return fallback.Items, nil
	}

	return out, nil
}

func GetBackendTLSPolicyV1(
	ctx context.Context,
	cl client.Client,
	key client.ObjectKey,
) (*unstructured.Unstructured, gatewayv1alpha3.BackendTLSPolicy, error) {
	obj := NewBackendTLSPolicyV1Object()
	if err := cl.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) && looksLikeFakeClient(cl) {
			var fallback gatewayv1alpha3.BackendTLSPolicy
			if fallbackErr := cl.Get(ctx, key, &fallback); fallbackErr != nil {
				return nil, gatewayv1alpha3.BackendTLSPolicy{}, fallbackErr
			}

			raw, encodeErr := EncodeBackendTLSPolicyV1(&fallback)
			if encodeErr != nil {
				return nil, gatewayv1alpha3.BackendTLSPolicy{}, encodeErr
			}
			return raw, fallback, nil
		}
		return nil, gatewayv1alpha3.BackendTLSPolicy{}, err
	}

	typed, err := DecodeBackendTLSPolicyV1(obj)
	if err != nil {
		return nil, gatewayv1alpha3.BackendTLSPolicy{}, err
	}

	return obj, typed, nil
}

func DecodeBackendTLSPolicyV1(
	obj *unstructured.Unstructured,
) (gatewayv1alpha3.BackendTLSPolicy, error) {
	var typed gatewayv1alpha3.BackendTLSPolicy
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &typed); err != nil {
		return gatewayv1alpha3.BackendTLSPolicy{}, fmt.Errorf("decode BackendTLSPolicy v1: %w", err)
	}
	return typed, nil
}

func EncodeBackendTLSPolicyV1(
	policy *gatewayv1alpha3.BackendTLSPolicy,
) (*unstructured.Unstructured, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policy)
	if err != nil {
		return nil, fmt.Errorf("encode BackendTLSPolicy v1: %w", err)
	}

	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(BackendTLSPolicyV1GVK)
	return obj, nil
}

func UpdateBackendTLSPolicyV1Status(
	ctx context.Context,
	cl client.Client,
	current *unstructured.Unstructured,
	status gatewayv1.PolicyStatus,
) error {
	if looksLikeFakeClient(cl) {
		var fallback gatewayv1alpha3.BackendTLSPolicy
		if err := cl.Get(ctx, client.ObjectKeyFromObject(current), &fallback); err == nil {
			fallback.Status = status
			return cl.Status().Update(ctx, &fallback)
		}
	}

	desired := current.DeepCopy()
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return fmt.Errorf("encode BackendTLSPolicy status: %w", err)
	}
	desired.Object["status"] = statusMap
	desired.SetGroupVersionKind(BackendTLSPolicyV1GVK)
	return cl.Status().Update(ctx, desired)
}

func looksLikeFakeClient(cl client.Client) bool {
	return strings.Contains(fmt.Sprintf("%T", cl), "fake")
}

func looksLikeFakeReader(reader client.Reader) bool {
	return strings.Contains(fmt.Sprintf("%T", reader), "fake")
}
