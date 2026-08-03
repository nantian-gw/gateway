package backends

import (
	"crypto/x509"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/backendtls"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

type translatedBackendTLSPolicy struct {
	backendSpecificity map[string]int
	validation         *ir.BackendTLSValidation
	policy             gatewayv1alpha3.BackendTLSPolicy
}

func BackendTLSValidationIndexWithIndexes(
	policies []gatewayv1alpha3.BackendTLSPolicy,
	indexes shared.TranslatorIndexes,
) map[string]*ir.BackendTLSValidation {
	translations := make([]translatedBackendTLSPolicy, 0, len(policies))
	owners := make(map[string]int)

	for _, policy := range policies {
		validation, ok := translateBackendTLSPolicyValidationWithIndexes(policy, indexes)
		if !ok {
			continue
		}

		backendSpecificity, ok := backendTLSPolicyBackendSpecificityWithIndexes(policy, indexes)
		if !ok || len(backendSpecificity) == 0 {
			continue
		}

		translations = append(translations, translatedBackendTLSPolicy{
			backendSpecificity: backendSpecificity,
			validation:         validation,
			policy:             policy,
		})
		translationIndex := len(translations) - 1
		for backendKey, specificity := range backendSpecificity {
			currentOwner, exists := owners[backendKey]
			if !exists || backendTLSPolicyAssignmentPrecedes(
				policy,
				specificity,
				translations[currentOwner].policy,
				translations[currentOwner].backendSpecificity[backendKey],
			) {
				owners[backendKey] = translationIndex
			}
		}
	}

	out := make(map[string]*ir.BackendTLSValidation, len(owners))
	for backendKey, ownerIndex := range owners {
		item := *translations[ownerIndex].validation
		out[backendKey] = &item
	}

	return out
}

func translateBackendTLSPolicyValidationWithIndexes(
	policy gatewayv1alpha3.BackendTLSPolicy,
	indexes shared.TranslatorIndexes,
) (*ir.BackendTLSValidation, bool) {
	validation := policy.Spec.Validation
	if validation.Hostname == "" {
		return nil, false
	}

	options, err := backendtls.ParseOptions(policy.Spec.Options)
	if err != nil {
		return nil, false
	}

	subjectAltNames, ok := backendTLSPolicySubjectAltNames(validation)
	if !ok {
		return nil, false
	}

	if len(validation.CACertificateRefs) > 0 {
		if validation.WellKnownCACertificates != nil {
			return nil, false
		}

		caPEMs, _ := backendTLSPolicyCAPEMsWithIndexes(indexes, policy.Namespace, validation.CACertificateRefs)

		return &ir.BackendTLSValidation{
			Hostname:        string(validation.Hostname),
			CAPEMs:          caPEMs,
			SubjectAltNames: subjectAltNames,
			MinVersion:      options.MinVersion,
			MaxVersion:      options.MaxVersion,
		}, true
	}

	if validation.WellKnownCACertificates == nil ||
		*validation.WellKnownCACertificates != gatewayv1.WellKnownCACertificatesSystem {
		return nil, false
	}

	return &ir.BackendTLSValidation{
		Hostname:        string(validation.Hostname),
		UseSystemCAs:    true,
		SubjectAltNames: subjectAltNames,
		MinVersion:      options.MinVersion,
		MaxVersion:      options.MaxVersion,
	}, true
}

func backendTLSPolicySubjectAltNames(
	validation gatewayv1.BackendTLSPolicyValidation,
) ([]ir.BackendSubjectName, bool) {
	items, err := backendtls.ParseSubjectAltNames(validation.SubjectAltNames)
	if err != nil {
		return nil, false
	}

	out := make([]ir.BackendSubjectName, 0, len(items))
	for _, item := range items {
		out = append(out, ir.BackendSubjectName{
			Type:  item.Type,
			Value: item.Value,
		})
	}
	return out, true
}

func backendTLSPolicyCAPEMsWithIndexes(
	indexes shared.TranslatorIndexes,
	namespace string,
	refs []gatewayv1.LocalObjectReference,
) ([]string, bool) {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if string(ref.Group) != "" {
			continue
		}

		kind := string(ref.Kind)
		if kind == "" {
			kind = "ConfigMap"
		}

		var caPEM string
		switch kind {
		case "ConfigMap":
			caPEM = indexes.ConfigMapCAPEM(namespace, string(ref.Name))
		case "Secret":
			caPEM = indexes.SecretCAPEM(namespace, string(ref.Name))
		default:
			continue
		}

		if !validBackendTLSPolicyCAPEM(caPEM) {
			continue
		}
		out = append(out, caPEM)
	}

	return out, len(out) > 0
}

func validBackendTLSPolicyCAPEM(value string) bool {
	if value == "" {
		return false
	}

	pool := x509.NewCertPool()
	return pool.AppendCertsFromPEM([]byte(value))
}

func backendTLSPolicyBackendSpecificityWithIndexes(
	policy gatewayv1alpha3.BackendTLSPolicy,
	indexes shared.TranslatorIndexes,
) (map[string]int, bool) {
	keys := make(map[string]int)
	for _, targetRef := range policy.Spec.TargetRefs {
		group := string(targetRef.Group)
		kind := string(targetRef.Kind)
		specificity := 0
		if targetRef.SectionName != nil {
			specificity = 1
		}

		switch {
		case group == "" && kind == "Service":
			service, ok := indexes.Service(policy.Namespace, string(targetRef.Name))
			if !ok {
				continue
			}
			targetKeys, ok := serviceBackendKeys(
				policy.Namespace,
				service.Name,
				service.Spec.Ports,
				targetRef.SectionName,
			)
			if !ok {
				continue
			}
			for _, key := range targetKeys {
				currentSpecificity, exists := keys[key]
				if !exists || specificity > currentSpecificity {
					keys[key] = specificity
				}
			}
		case group == mcsv1alpha1.GroupName && kind == "ServiceImport":
			serviceImport, ok := indexes.ServiceImport(policy.Namespace, string(targetRef.Name))
			if !ok {
				continue
			}
			targetKeys, ok := serviceImportBackendKeys(
				policy.Namespace,
				serviceImport.Name,
				serviceImport.Spec.Ports,
				targetRef.SectionName,
			)
			if !ok {
				continue
			}
			for _, key := range targetKeys {
				currentSpecificity, exists := keys[key]
				if !exists || specificity > currentSpecificity {
					keys[key] = specificity
				}
			}
		default:
			continue
		}
	}

	return keys, len(keys) > 0
}

func backendTLSPolicyAssignmentPrecedes(
	left gatewayv1alpha3.BackendTLSPolicy,
	leftSpecificity int,
	right gatewayv1alpha3.BackendTLSPolicy,
	rightSpecificity int,
) bool {
	if leftSpecificity != rightSpecificity {
		return leftSpecificity > rightSpecificity
	}
	return backendtls.PolicyPrecedes(left, right)
}

func serviceBackendKeys(
	namespace string,
	name string,
	ports []corev1.ServicePort,
	sectionName *gatewayv1.SectionName,
) ([]string, bool) {
	keys := make([]string, 0, len(ports))
	for _, port := range ports {
		if sectionName != nil && port.Name != string(*sectionName) {
			continue
		}
		keys = append(keys, backendClusterKey(namespace, name, port.Port))
	}

	if sectionName != nil && len(keys) == 0 {
		return nil, false
	}

	return keys, len(keys) > 0
}

func serviceImportBackendKeys(
	namespace string,
	name string,
	ports []mcsv1alpha1.ServicePort,
	sectionName *gatewayv1.SectionName,
) ([]string, bool) {
	keys := make([]string, 0, len(ports))
	for _, port := range ports {
		if sectionName != nil && port.Name != string(*sectionName) {
			continue
		}
		keys = append(keys, backendClusterKey(namespace, name, port.Port))
	}

	if sectionName != nil && len(keys) == 0 {
		return nil, false
	}

	return keys, len(keys) > 0
}

func backendClusterKey(namespace, name string, port int32) string {
	return namespace + "/" + name + ":" + strconv.Itoa(int(port))
}

func compactStrings(items []string) []string {
	if len(items) < 2 {
		return items
	}

	out := items[:1]
	for _, item := range items[1:] {
		if item == out[len(out)-1] {
			continue
		}
		out = append(out, item)
	}
	return out
}
