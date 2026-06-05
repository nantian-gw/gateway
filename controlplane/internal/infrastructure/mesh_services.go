package infrastructure

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/mesh"
)

type frontendExposureMode int

const (
	frontendExposureStrictCurrent frontendExposureMode = iota
	frontendExposurePreferStable
)

func (r *Reconciler) reconcileMeshServices(ctx context.Context) error {
	_, _, eligibleDataplanePods, err := r.loadFrontendEligibleDataplanePods(ctx)
	if err != nil {
		return err
	}
	return r.reconcileMeshServicesWithPods(ctx, eligibleDataplanePods)
}

func (r *Reconciler) reconcileMeshServicesWithPods(ctx context.Context, eligibleDataplanePods []corev1.Pod) error {
	meshParentKeys, err := r.loadMeshServiceParents(ctx)
	if err != nil {
		return err
	}
	serviceState, err := loadMeshServiceState(ctx, r.client, meshParentKeys)
	if err != nil {
		return err
	}

	frontends := mesh.ExpandServiceFrontends(
		meshServices(serviceState),
		meshParentKeys,
	)
	frontendsByService := make(map[string][]mesh.ServiceFrontendPort)
	for _, frontend := range frontends {
		key := frontend.Namespace + "/" + frontend.Name
		frontendsByService[key] = append(frontendsByService[key], frontend)
	}

	endpointState, err := loadMeshServiceEndpointState(
		ctx,
		r.client,
		unionServiceKeys(serviceKeySetFromFrontends(frontendsByService), serviceState.managedServiceKeys),
	)
	if err != nil {
		return err
	}

	desiredKeys := make(map[string]struct{}, len(frontendsByService))
	desiredMeshServices := make(map[string]*corev1.Service, len(frontendsByService))
	meshEndpointsByFamily := meshDataplaneEndpoints(eligibleDataplanePods)
	for key, serviceFrontends := range frontendsByService {
		current, ok := serviceState.servicesByKey[key]
		if !ok {
			continue
		}

		desiredKeys[key] = struct{}{}
		shadow := serviceState.shadowByOriginal[key]
		source := sourceService(current, shadow)
		shadowName := mesh.ShadowServiceName(current.Namespace, current.Name)
		if err := applyService(
			ctx,
			r.client,
			serviceOrEmpty(shadow),
			desiredShadowService(shadow, current.Namespace, current.Name, shadowName, source),
		); err != nil {
			return err
		}

		if err := applyService(
			ctx,
			r.client,
			&current,
			desiredMeshFrontendService(current, shadowName, source, serviceFrontends),
		); err != nil {
			return err
		}
		updatedService, err := mustGetService(
			ctx,
			r.client,
			client.ObjectKey{Namespace: current.Namespace, Name: current.Name},
		)
		if err != nil {
			return err
		}
		if err := deleteServiceEndpoints(ctx, r.client, endpointState.endpointsByService[key]); err != nil {
			return err
		}
		if err := deleteMeshEndpointSlices(ctx, r.client, endpointState.foreignEndpointSlicesByService[key]); err != nil {
			return err
		}

		desiredMeshServices[key] = updatedService
	}

	for key := range serviceState.managedServiceKeys {
		current := serviceState.servicesByKey[key]
		if _, ok := desiredKeys[key]; ok {
			continue
		}

		shadow := serviceState.shadowByOriginal[key]
		if shadow.Name != "" {
			if err := applyService(
				ctx,
				r.client,
				&current,
				restoreMeshService(current, shadow),
			); err != nil {
				return err
			}
			if err := r.client.Delete(ctx, &shadow); client.IgnoreNotFound(err) != nil {
				return err
			}
			if err := deleteMeshEndpointSlices(ctx, r.client, endpointState.managedEndpointSlicesByService[key]); err != nil {
				return err
			}
			delete(serviceState.shadowByOriginal, key)
			continue
		}

		if err := applyService(
			ctx,
			r.client,
			&current,
			stripMeshServiceAnnotations(current),
		); err != nil {
			return err
		}
		if err := deleteMeshEndpointSlices(ctx, r.client, endpointState.managedEndpointSlicesByService[key]); err != nil {
			return err
		}
	}

	for key, shadow := range serviceState.shadowByOriginal {
		if _, ok := desiredKeys[key]; ok {
			continue
		}
		if err := r.client.Delete(ctx, &shadow); client.IgnoreNotFound(err) != nil {
			return err
		}
	}

	for key, service := range desiredMeshServices {
		if err := reconcileMeshEndpointSlicesFromDataplaneEndpoints(
			ctx,
			r.client,
			*service,
			meshEndpointsByFamily,
			endpointState.managedEndpointSlicesByService[key],
		); err != nil {
			return err
		}
	}

	for key, slices := range endpointState.managedEndpointSlicesByService {
		if _, ok := desiredMeshServices[key]; ok {
			continue
		}
		if err := deleteMeshEndpointSlices(ctx, r.client, slices); err != nil {
			return err
		}
	}

	return nil
}

func serviceKeySetFromFrontends(frontendsByService map[string][]mesh.ServiceFrontendPort) map[string]struct{} {
	keys := make(map[string]struct{}, len(frontendsByService))
	for key := range frontendsByService {
		keys[key] = struct{}{}
	}
	return keys
}

func unionServiceKeys(sets ...map[string]struct{}) map[string]struct{} {
	total := 0
	for _, set := range sets {
		total += len(set)
	}

	out := make(map[string]struct{}, total)
	for _, set := range sets {
		for key := range set {
			out[key] = struct{}{}
		}
	}
	return out
}

func (r *Reconciler) frontendEligibleDataplanePods(
	ctx context.Context,
	pods []corev1.Pod,
	mode frontendExposureMode,
) []corev1.Pod {
	if r.store == nil || r.nodes == nil {
		return pods
	}

	statuses := r.nodes.List(ctx)
	if len(statuses) == 0 {
		return nil
	}

	snapshot := r.store.Current()
	currentVersion := ""
	if snapshot != nil {
		currentVersion = snapshot.ID
	}

	currentNodeIDs := make(map[string]struct{}, len(statuses))
	stableVersionCohorts := make(map[string]int, len(statuses))
	stableVersionLastSeen := make(map[string]time.Time, len(statuses))
	for _, status := range statuses {
		if !status.Ready || !status.Connected {
			continue
		}

		lastSentVersion := strings.TrimSpace(status.LastSentVersion)
		lastAckVersion := strings.TrimSpace(status.LastAckVersion)
		if currentVersion != "" &&
			lastSentVersion == currentVersion &&
			lastAckVersion == currentVersion &&
			!status.RejectsVersion(currentVersion) {
			currentNodeIDs[status.NodeID] = struct{}{}
		}

		stableVersion := lastAckVersion
		if stableVersion == "" || status.RejectsVersion(stableVersion) {
			continue
		}
		stableVersionCohorts[stableVersion]++
		if status.LastSeenAt.After(stableVersionLastSeen[stableVersion]) {
			stableVersionLastSeen[stableVersion] = status.LastSeenAt
		}
	}

	eligibleNodeIDs := currentNodeIDs
	if len(eligibleNodeIDs) == 0 && mode == frontendExposurePreferStable {
		stableVersion := preferredStableAckVersion(stableVersionCohorts, stableVersionLastSeen)
		if stableVersion != "" {
			eligibleNodeIDs = eligibleNodeIDsForVersion(statuses, stableVersion)
			if len(eligibleNodeIDs) > 0 && r.logger != nil && currentVersion != "" {
				r.logger.Warn(
					"no dataplane node has acknowledged the current snapshot; exposing the last stable frontend cohort",
					"current_version",
					currentVersion,
					"stable_version",
					stableVersion,
					"eligible_nodes",
					len(eligibleNodeIDs),
				)
			}
		}
	}
	if len(eligibleNodeIDs) == 0 {
		return nil
	}

	filtered := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if _, ok := eligibleNodeIDs[pod.Name]; !ok {
			continue
		}
		filtered = append(filtered, pod)
	}
	return filtered
}

func (r *Reconciler) frontendEligibleDataplanePodSets(
	ctx context.Context,
	pods []corev1.Pod,
) ([]corev1.Pod, []corev1.Pod, []corev1.Pod) {
	current := r.frontendEligibleDataplanePods(ctx, pods, frontendExposureStrictCurrent)
	return current, current, current
}

func eligibleNodeIDsForVersion(statuses []ir.NodeStatus, version string) map[string]struct{} {
	if version == "" {
		return nil
	}

	out := make(map[string]struct{})
	for _, status := range statuses {
		if !status.Ready || !status.Connected || status.LastAckVersion != version || status.RejectsVersion(version) {
			continue
		}
		out[status.NodeID] = struct{}{}
	}
	return out
}

func preferredStableAckVersion(
	cohorts map[string]int,
	lastSeen map[string]time.Time,
) string {
	bestVersion := ""
	bestCount := 0
	bestSeen := time.Time{}

	for version, count := range cohorts {
		if count > bestCount {
			bestVersion = version
			bestCount = count
			bestSeen = lastSeen[version]
			continue
		}

		if count == bestCount && lastSeen[version].After(bestSeen) {
			bestVersion = version
			bestSeen = lastSeen[version]
		}
	}

	return bestVersion
}

func collectMeshServiceParents(
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
	tcpRoutes []gatewayv1alpha2.TCPRoute,
	udpRoutes []gatewayv1alpha2.UDPRoute,
	tlsRoutes []gatewayv1alpha2.TLSRoute,
) []mesh.ServiceParentKey {
	out := make([]mesh.ServiceParentKey, 0)

	for _, route := range httpRoutes {
		out = append(out, serviceParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range grpcRoutes {
		out = append(out, serviceParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range tcpRoutes {
		out = append(out, serviceParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range udpRoutes {
		out = append(out, serviceParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}
	for _, route := range tlsRoutes {
		out = append(out, serviceParentKeys(route.Spec.ParentRefs, route.Namespace)...)
	}

	return out
}

func serviceParentKeys(
	parentRefs []gatewayv1.ParentReference,
	defaultNamespace string,
) []mesh.ServiceParentKey {
	out := make([]mesh.ServiceParentKey, 0, len(parentRefs))
	for _, parentRef := range parentRefs {
		if key, ok := mesh.ParentServiceRef(parentRef, defaultNamespace); ok {
			out = append(out, key)
		}
	}
	return out
}

func desiredShadowService(
	current corev1.Service,
	namespace string,
	name string,
	shadowName string,
	source corev1.Service,
) *corev1.Service {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shadowName,
			Namespace: namespace,
			Labels: map[string]string{
				managedByLabel:                     managedByValue,
				mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
				mesh.OriginalServiceNameLabel:      name,
				mesh.OriginalServiceNamespaceLabel: namespace,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     source.Spec.Type,
			Selector: cloneStringMap(source.Spec.Selector),
			Ports:    cloneServicePorts(source.Spec.Ports),
		},
	}

	if current.Name != "" {
		desired.ResourceVersion = current.ResourceVersion
		desired.Spec.ClusterIP = current.Spec.ClusterIP
		desired.Spec.ClusterIPs = append([]string(nil), current.Spec.ClusterIPs...)
		desired.Spec.IPFamilies = append([]corev1.IPFamily(nil), current.Spec.IPFamilies...)
		desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		desired.Spec.InternalTrafficPolicy = current.Spec.InternalTrafficPolicy
		desired.Spec.SessionAffinity = current.Spec.SessionAffinity
	} else if source.Spec.ClusterIP == corev1.ClusterIPNone {
		desired.Spec.ClusterIP = corev1.ClusterIPNone
	}

	return desired
}

func desiredMeshFrontendService(
	current corev1.Service,
	shadowName string,
	source corev1.Service,
	frontends []mesh.ServiceFrontendPort,
) *corev1.Service {
	desired := current.DeepCopy()
	desired.Labels = cloneStringMap(current.Labels)
	if desired.Labels == nil {
		desired.Labels = make(map[string]string)
	}
	desired.Labels[managedByLabel] = managedByValue
	desired.Labels[serviceRoleLabel] = serviceRoleMeshFrontend
	desired.Annotations = cloneStringMap(current.Annotations)
	if desired.Annotations == nil {
		desired.Annotations = make(map[string]string)
	}
	desired.Annotations[mesh.ManagedServiceAnnotation] = "true"
	desired.Annotations[mesh.ShadowServiceAnnotation] = shadowName
	desired.Spec.Selector = nil
	desired.Spec.Ports = remapMeshFrontendPorts(source.Spec.Ports, frontends)
	return desired
}

func restoreMeshService(current corev1.Service, shadow corev1.Service) *corev1.Service {
	desired := current.DeepCopy()
	desired.Labels = pruneMeshServiceLabels(current.Labels)
	desired.Annotations = pruneMeshAnnotations(current.Annotations)
	desired.Spec.Type = shadow.Spec.Type
	desired.Spec.Selector = cloneStringMap(shadow.Spec.Selector)
	desired.Spec.Ports = cloneServicePorts(shadow.Spec.Ports)
	return desired
}

func stripMeshServiceAnnotations(current corev1.Service) *corev1.Service {
	desired := current.DeepCopy()
	desired.Labels = pruneMeshServiceLabels(current.Labels)
	desired.Annotations = pruneMeshAnnotations(current.Annotations)
	return desired
}

func remapMeshFrontendPorts(
	ports []corev1.ServicePort,
	frontends []mesh.ServiceFrontendPort,
) []corev1.ServicePort {
	assignments := make(map[int32]mesh.ServiceFrontendPort, len(frontends))
	for _, frontend := range frontends {
		assignments[frontend.ServicePort] = frontend
	}

	out := cloneServicePorts(ports)
	for idx := range out {
		frontend, ok := assignments[out[idx].Port]
		if !ok {
			continue
		}
		out[idx].TargetPort = intstr.FromInt(int(frontend.ListenPort))
	}
	return out
}

func cloneServicePorts(ports []corev1.ServicePort) []corev1.ServicePort {
	out := make([]corev1.ServicePort, len(ports))
	copy(out, ports)
	return out
}

func pruneMeshAnnotations(annotations map[string]string) map[string]string {
	out := cloneStringMap(annotations)
	delete(out, mesh.ManagedServiceAnnotation)
	delete(out, mesh.ShadowServiceAnnotation)
	if len(out) == 0 {
		return nil
	}
	return out
}

func pruneMeshServiceLabels(labels map[string]string) map[string]string {
	out := cloneStringMap(labels)
	delete(out, managedByLabel)
	delete(out, serviceRoleLabel)
	if len(out) == 0 {
		return nil
	}
	return out
}

func serviceOrEmpty(service corev1.Service) *corev1.Service {
	if service.Name == "" {
		return &corev1.Service{}
	}
	return &service
}

func sourceService(current corev1.Service, shadow corev1.Service) corev1.Service {
	if current.Annotations[mesh.ManagedServiceAnnotation] == "true" && shadow.Name != "" {
		return shadow
	}
	return current
}

func deleteServiceEndpoints(
	ctx context.Context,
	cl client.Client,
	endpoints corev1.Endpoints,
) error {
	if endpoints.Name == "" {
		return nil
	}
	return client.IgnoreNotFound(cl.Delete(ctx, &endpoints))
}
