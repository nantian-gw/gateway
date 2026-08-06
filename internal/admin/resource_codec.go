package admin

import (
	jsoniter "github.com/json-iterator/go"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	k8syaml "sigs.k8s.io/yaml"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
)

func managedResourceFromObject(spec resourceKindSpec, obj client.Object) (ManagedResource, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return ManagedResource{}, fmt.Errorf("convert resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	raw["apiVersion"] = spec.descriptor.APIVersion
	raw["kind"] = spec.descriptor.Kind
	normalizeResourceMap(raw)

	return ManagedResource{
		APIVersion:        spec.descriptor.APIVersion,
		Kind:              spec.descriptor.Kind,
		Namespace:         obj.GetNamespace(),
		Name:              obj.GetName(),
		UID:               string(obj.GetUID()),
		Generation:        obj.GetGeneration(),
		CreationTimestamp: obj.GetCreationTimestamp().UTC(),
		Labels:            cloneStringMap(obj.GetLabels()),
		Annotations:       cloneStringMap(filterSystemAnnotations(obj.GetAnnotations())),
		Resource:          raw,
	}, nil
}

func decodeManagedResource(raw []byte, expectedKind string) (resourceKindSpec, client.Object, error) {
	jsonBody, err := k8syaml.YAMLToJSON(raw)
	if err != nil {
		return resourceKindSpec{}, nil, errInvalidRequest(fmt.Sprintf("decode resource manifest: %v", err))
	}

	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := jsoniter.Unmarshal(jsonBody, &envelope); err != nil {
		return resourceKindSpec{}, nil, errInvalidRequest(fmt.Sprintf("decode resource kind: %v", err))
	}

	kind := strings.TrimSpace(envelope.Kind)
	if expectedKind != "" {
		if kind == "" {
			kind = expectedKind
		} else if !IsSameResourceKind(kind, expectedKind) {
			return resourceKindSpec{}, nil, errInvalidRequest("resource kind does not match request path")
		}
	}
	if kind == "" {
		return resourceKindSpec{}, nil, errInvalidRequest("resource kind is required")
	}

	spec, err := resourceKindSpecFor(kind)
	if err != nil {
		return resourceKindSpec{}, nil, err
	}

	obj := spec.newObject()
	if err := jsoniter.Unmarshal(jsonBody, obj); err != nil {
		return resourceKindSpec{}, nil, errInvalidRequest(fmt.Sprintf("decode resource manifest: %v", err))
	}

	gvk := spec.gvk()
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return spec, obj, nil
}

func normalizeResourceMap(raw map[string]any) {
	metadata, ok := raw["metadata"].(map[string]any)
	if ok {
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "uid")
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func filterSystemAnnotations(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string, len(values))
	for key, value := range values {
		if strings.HasPrefix(key, "kubectl.kubernetes.io/") {
			continue
		}
		out[key] = value
	}

	return out
}

func metaExtractList(list client.ObjectList) ([]client.Object, error) {
	items, err := metaExtractObjects(list)
	if err != nil {
		return nil, fmt.Errorf("extract resource list: %w", err)
	}

	return items, nil
}

func metaExtractObjects(list client.ObjectList) ([]client.Object, error) {
	switch typed := list.(type) {
	case *gatewayv1.GatewayList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1.GatewayClassList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1.HTTPRouteList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1.GRPCRouteList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1alpha2.TCPRouteList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1alpha2.UDPRouteList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1alpha2.TLSRouteList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *backend.BackendLBPolicyList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *unstructured.UnstructuredList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *gatewayv1beta1.ReferenceGrantList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	case *mcsv1alpha1.ServiceImportList:
		out := make([]client.Object, 0, len(typed.Items))
		for i := range typed.Items {
			out = append(out, &typed.Items[i])
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported resource list type %T", list)
	}
}
