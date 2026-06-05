package status

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
)

const (
	gatewayAPIBundleVersionAnnotation = "gateway.networking.k8s.io/bundle-version"
	minSupportedGatewayAPIVersion     = "v1.5.1"
	maxSupportedGatewayAPIVersion     = "v1.5.1"
)

type gatewayAPISemver struct {
	major int
	minor int
	patch int
}

var (
	minSupportedGatewayAPISemver = gatewayAPISemver{major: 1, minor: 5, patch: 1}
	maxSupportedGatewayAPISemver = gatewayAPISemver{major: 1, minor: 5, patch: 1}
)

type gatewayClassStatusSupportResolver struct {
	reconciler *Reconciler
	loaded     bool
	crds       []apiextensionsv1.CustomResourceDefinition
	features   []gatewayv1.SupportedFeature
	err        error
}

func newGatewayClassStatusSupportResolver(reconciler *Reconciler) *gatewayClassStatusSupportResolver {
	return &gatewayClassStatusSupportResolver{reconciler: reconciler}
}

func (r *Reconciler) gatewayClassStatusSupport(
	ctx context.Context,
	generation int64,
) (conditionSpec, []gatewayv1.SupportedFeature, error) {
	return newGatewayClassStatusSupportResolver(r).resolve(ctx, generation)
}

func (r *gatewayClassStatusSupportResolver) resolve(
	ctx context.Context,
	generation int64,
) (conditionSpec, []gatewayv1.SupportedFeature, error) {
	if r == nil || r.reconciler == nil {
		return conditionSpec{}, nil, nil
	}

	if !r.loaded {
		r.loaded = true
		var crds apiextensionsv1.CustomResourceDefinitionList
		if err := r.reconciler.reader.List(ctx, &crds); err != nil {
			r.err = err
		} else {
			r.crds = crds.Items
			r.features = gatewayapi.SupportedFeatures()
		}
	}
	if r.err != nil {
		return conditionSpec{}, nil, r.err
	}

	return gatewayClassSupportedVersionCondition(generation, r.crds), append([]gatewayv1.SupportedFeature(nil), r.features...), nil
}

func gatewayClassSupportedVersionCondition(
	generation int64,
	crds []apiextensionsv1.CustomResourceDefinition,
) conditionSpec {
	condition := conditionSpec{
		Type:               string(gatewayv1.GatewayClassConditionStatusSupportedVersion),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayClassReasonSupportedVersion),
		Message:            "Gateway API CRD bundle versions are supported",
		ObservedGeneration: generation,
	}

	detectedVersions := make(map[string]struct{})
	missingAnnotations := make([]string, 0)
	unsupportedVersions := make([]string, 0)
	seenGatewayAPICRD := false

	for _, crd := range crds {
		if crd.Spec.Group != gatewayv1.GroupName {
			continue
		}
		seenGatewayAPICRD = true

		version := strings.TrimSpace(crd.Annotations[gatewayAPIBundleVersionAnnotation])
		if version == "" {
			missingAnnotations = append(missingAnnotations, crd.Name)
			continue
		}
		detectedVersions[version] = struct{}{}
		if !gatewayAPIBundleVersionSupported(version) {
			unsupportedVersions = append(unsupportedVersions, fmt.Sprintf("%s=%s", crd.Name, version))
		}
	}

	sort.Strings(missingAnnotations)
	sort.Strings(unsupportedVersions)

	switch {
	case !seenGatewayAPICRD:
		condition.Status = metav1.ConditionFalse
		condition.Reason = string(gatewayv1.GatewayClassReasonUnsupportedVersion)
		condition.Message = "No Gateway API CRDs were detected while evaluating supported versions"
	case len(missingAnnotations) > 0:
		condition.Status = metav1.ConditionFalse
		condition.Reason = string(gatewayv1.GatewayClassReasonUnsupportedVersion)
		condition.Message = fmt.Sprintf(
			"Gateway API CRDs are missing the %s annotation: %s",
			gatewayAPIBundleVersionAnnotation,
			strings.Join(missingAnnotations, ", "),
		)
	case len(unsupportedVersions) > 0:
		condition.Status = metav1.ConditionFalse
		condition.Reason = string(gatewayv1.GatewayClassReasonUnsupportedVersion)
		condition.Message = fmt.Sprintf(
			"Unsupported Gateway API CRD bundle versions detected: %s (supported range: %s - %s)",
			strings.Join(unsupportedVersions, ", "),
			minSupportedGatewayAPIVersion,
			maxSupportedGatewayAPIVersion,
		)
	default:
		versions := make([]string, 0, len(detectedVersions))
		for version := range detectedVersions {
			versions = append(versions, version)
		}
		sort.Strings(versions)
		if len(versions) > 0 {
			condition.Message = fmt.Sprintf(
				"Gateway API CRD bundle versions are supported: %s",
				strings.Join(versions, ", "),
			)
		}
	}

	return condition
}

func gatewayAPIBundleVersionSupported(version string) bool {
	parsed, ok := parseGatewayAPISemver(version)
	if !ok {
		return false
	}

	return parsed.compare(minSupportedGatewayAPISemver) >= 0 &&
		parsed.compare(maxSupportedGatewayAPISemver) <= 0
}

func parseGatewayAPISemver(raw string) (gatewayAPISemver, bool) {
	version := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if version == "" {
		return gatewayAPISemver{}, false
	}

	parts := strings.SplitN(version, ".", 4)
	if len(parts) != 3 {
		return gatewayAPISemver{}, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return gatewayAPISemver{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return gatewayAPISemver{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return gatewayAPISemver{}, false
	}

	return gatewayAPISemver{
		major: major,
		minor: minor,
		patch: patch,
	}, true
}

func (v gatewayAPISemver) compare(other gatewayAPISemver) int {
	switch {
	case v.major != other.major:
		return compareInts(v.major, other.major)
	case v.minor != other.minor:
		return compareInts(v.minor, other.minor)
	default:
		return compareInts(v.patch, other.patch)
	}
}

func compareInts(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
