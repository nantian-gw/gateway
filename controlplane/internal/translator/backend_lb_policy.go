package translator

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/controlplane/internal/backendlb"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

type translatedBackendLBPolicy struct {
	backendKeys        []string
	sessionPersistence *ir.SessionPersistencePolicy
	loadBalancing      *ir.LoadBalancingPolicy
	policy             backendlbv1alpha2.BackendLBPolicy
}

type backendLBPolicyIndexes struct {
	sessionPersistence map[string]*ir.SessionPersistencePolicy
	loadBalancing      map[string]*ir.LoadBalancingPolicy
}

func buildBackendLBPolicyIndexes(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	policies []backendlbv1alpha2.BackendLBPolicy,
) backendLBPolicyIndexes {
	return buildBackendLBPolicyIndexesWithIndexes(
		policies,
		newTranslatorIndexes(services, serviceImports, nil, nil, nil, nil),
	)
}

func buildBackendLBPolicyIndexesWithIndexes(
	policies []backendlbv1alpha2.BackendLBPolicy,
	indexes translatorIndexes,
) backendLBPolicyIndexes {
	translations := make([]translatedBackendLBPolicy, 0, len(policies))
	owners := make(map[string]int)

	for _, policy := range policies {
		sessionPersistence := backendSessionPersistence(
			policy.Namespace,
			policy.Name,
			policy.Spec.SessionPersistence,
		)
		loadBalancing := backendLoadBalancing(policy.Spec.LoadBalancing)
		if sessionPersistence == nil && loadBalancing == nil {
			continue
		}

		backendKeys, ok := backendLBPolicyBackendKeysWithIndexes(policy, indexes)
		if !ok || len(backendKeys) == 0 {
			continue
		}

		translations = append(translations, translatedBackendLBPolicy{
			backendKeys:        backendKeys,
			sessionPersistence: sessionPersistence,
			loadBalancing:      loadBalancing,
			policy:             policy,
		})
		translationIndex := len(translations) - 1
		for _, backendKey := range backendKeys {
			currentOwner, exists := owners[backendKey]
			if !exists || backendlb.PolicyPrecedes(policy, translations[currentOwner].policy) {
				owners[backendKey] = translationIndex
			}
		}
	}

	sessionPersistence := make(map[string]*ir.SessionPersistencePolicy, len(owners))
	loadBalancing := make(map[string]*ir.LoadBalancingPolicy, len(owners))
	for backendKey, ownerIndex := range owners {
		if item := translations[ownerIndex].sessionPersistence; item != nil {
			copyItem := *item
			sessionPersistence[backendKey] = &copyItem
		}
		if item := translations[ownerIndex].loadBalancing; item != nil {
			copyItem := *item
			if item.ConsistentHash != nil {
				hashCopy := *item.ConsistentHash
				copyItem.ConsistentHash = &hashCopy
			}
			loadBalancing[backendKey] = &copyItem
		}
	}

	return backendLBPolicyIndexes{
		sessionPersistence: sessionPersistence,
		loadBalancing:      loadBalancing,
	}
}

func backendLBPolicyBackendKeys(
	policy backendlbv1alpha2.BackendLBPolicy,
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
) ([]string, bool) {
	return backendLBPolicyBackendKeysWithIndexes(
		policy,
		newTranslatorIndexes(services, serviceImports, nil, nil, nil, nil),
	)
}

func backendLBPolicyBackendKeysWithIndexes(
	policy backendlbv1alpha2.BackendLBPolicy,
	indexes translatorIndexes,
) ([]string, bool) {
	keys := make([]string, 0)
	for _, targetRef := range policy.Spec.TargetRefs {
		group := string(targetRef.Group)
		kind := string(targetRef.Kind)

		switch {
		case group == "" && kind == "Service":
			service, ok := indexes.service(policy.Namespace, string(targetRef.Name))
			if !ok {
				return nil, false
			}
			targetKeys, ok := serviceBackendKeys(
				policy.Namespace,
				service.Name,
				service.Spec.Ports,
				nil,
			)
			if !ok {
				return nil, false
			}
			keys = append(keys, targetKeys...)
		case group == mcsv1alpha1.GroupName && kind == "ServiceImport":
			serviceImport, ok := indexes.serviceImport(policy.Namespace, string(targetRef.Name))
			if !ok {
				return nil, false
			}
			targetKeys, ok := serviceImportBackendKeys(
				policy.Namespace,
				serviceImport.Name,
				serviceImport.Spec.Ports,
				nil,
			)
			if !ok {
				return nil, false
			}
			keys = append(keys, targetKeys...)
		default:
			return nil, false
		}
	}

	sort.Strings(keys)
	return compactStrings(keys), true
}
