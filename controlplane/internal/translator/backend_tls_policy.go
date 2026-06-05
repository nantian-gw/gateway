package translator

import (
	"crypto/x509"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/controlplane/internal/backendtls"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

type translatedBackendTLSPolicy struct {
	backendSpecificity map[string]int
	validation         *ir.BackendTLSValidation
	policy             gatewayv1alpha3.BackendTLSPolicy
}

func backendTLSValidationIndex(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	configMaps []corev1.ConfigMap,
	policies []gatewayv1alpha3.BackendTLSPolicy,
) map[string]*ir.BackendTLSValidation {
	return backendTLSValidationIndexWithIndexes(
		policies,
		newTranslatorIndexes(services, serviceImports, nil, nil, configMaps, nil),
	)
}

func backendTLSValidationIndexWithIndexes(
	policies []gatewayv1alpha3.BackendTLSPolicy,
	indexes translatorIndexes,
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

func translateBackendTLSPolicyValidation(
	policy gatewayv1alpha3.BackendTLSPolicy,
	configMaps []corev1.ConfigMap,
) (*ir.BackendTLSValidation, bool) {
	return translateBackendTLSPolicyValidationWithIndexes(
		policy,
		newTranslatorIndexes(nil, nil, nil, nil, configMaps, nil),
	)
}

func translateBackendTLSPolicyValidationWithIndexes(
	policy gatewayv1alpha3.BackendTLSPolicy,
	indexes translatorIndexes,
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

		caPEMs, ok := backendTLSPolicyCAPEMsWithIndexes(indexes, policy.Namespace, validation.CACertificateRefs)
		if !ok || len(caPEMs) == 0 {
			return nil, false
		}

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

func backendTLSPolicyCAPEMs(
	configMaps []corev1.ConfigMap,
	namespace string,
	refs []gatewayv1.LocalObjectReference,
) ([]string, bool) {
	return backendTLSPolicyCAPEMsWithIndexes(
		newTranslatorIndexes(nil, nil, nil, nil, configMaps, nil),
		namespace,
		refs,
	)
}

func backendTLSPolicyCAPEMsWithIndexes(
	indexes translatorIndexes,
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
		if kind != "ConfigMap" {
			continue
		}

		caPEM := indexes.configMapCAPEM(namespace, string(ref.Name))
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

func backendTLSPolicyBackendKeys(
	policy gatewayv1alpha3.BackendTLSPolicy,
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
) ([]string, bool) {
	specificity, ok := backendTLSPolicyBackendSpecificityWithIndexes(
		policy,
		newTranslatorIndexes(services, serviceImports, nil, nil, nil, nil),
	)
	if !ok || len(specificity) == 0 {
		return nil, false
	}

	keys := make([]string, 0, len(specificity))
	for key := range specificity {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, true
}

func backendTLSPolicyBackendSpecificity(
	policy gatewayv1alpha3.BackendTLSPolicy,
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
) (map[string]int, bool) {
	return backendTLSPolicyBackendSpecificityWithIndexes(
		policy,
		newTranslatorIndexes(services, serviceImports, nil, nil, nil, nil),
	)
}

func backendTLSPolicyBackendSpecificityWithIndexes(
	policy gatewayv1alpha3.BackendTLSPolicy,
	indexes translatorIndexes,
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
			service, ok := indexes.service(policy.Namespace, string(targetRef.Name))
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
			serviceImport, ok := indexes.serviceImport(policy.Namespace, string(targetRef.Name))
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

func findServiceForPolicy(
	services []corev1.Service,
	namespace string,
	name string,
) (corev1.Service, bool) {
	for _, service := range services {
		if service.Namespace == namespace && service.Name == name {
			return service, true
		}
	}
	return corev1.Service{}, false
}

func findServiceImportForPolicy(
	serviceImports []mcsv1alpha1.ServiceImport,
	namespace string,
	name string,
) (mcsv1alpha1.ServiceImport, bool) {
	for _, serviceImport := range serviceImports {
		if serviceImport.Namespace == namespace && serviceImport.Name == name {
			return serviceImport, true
		}
	}
	return mcsv1alpha1.ServiceImport{}, false
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
	return fmt.Sprintf("%s/%s:%d", namespace, name, port)
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
