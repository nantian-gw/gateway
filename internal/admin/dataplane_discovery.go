package admin

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DataplaneAdminDiscoveryConfig struct {
	Namespace   string
	ServiceName string
	PortName    string
}

type DataplaneAdminEndpoint struct {
	NodeID  string `json:"nodeId"`
	Address string `json:"address"`
	URL     string `json:"url"`
	Ready   bool   `json:"ready"`
}

type DataplaneAdminDiscovery struct {
	client client.Client
	config DataplaneAdminDiscoveryConfig
}

func NewDataplaneAdminDiscovery(c client.Client, config DataplaneAdminDiscoveryConfig) *DataplaneAdminDiscovery {
	return &DataplaneAdminDiscovery{client: c, config: config}
}

func (d *DataplaneAdminDiscovery) List(ctx context.Context) ([]DataplaneAdminEndpoint, error) {
	var slices discoveryv1.EndpointSliceList
	if err := d.client.List(ctx, &slices,
		client.InNamespace(d.config.Namespace),
		client.MatchingLabels{"kubernetes.io/service-name": d.config.ServiceName},
	); err != nil {
		return nil, err
	}

	out := make([]DataplaneAdminEndpoint, 0)
	for i := range slices.Items {
		port, ok := endpointSlicePort(slices.Items[i], d.config.PortName)
		if !ok {
			continue
		}
		for _, endpoint := range slices.Items[i].Endpoints {
			if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
				continue
			}
			if len(endpoint.Addresses) == 0 {
				continue
			}
			nodeID := endpointNodeID(endpoint.TargetRef)
			address := fmt.Sprintf("%s:%d", endpoint.Addresses[0], port)
			out = append(out, DataplaneAdminEndpoint{
				NodeID:  nodeID,
				Address: address,
				URL:     "http://" + address,
				Ready:   true,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out, nil
}

func endpointSlicePort(slice discoveryv1.EndpointSlice, name string) (int32, bool) {
	for _, port := range slice.Ports {
		if port.Port == nil {
			continue
		}
		if name == "" || port.Name == nil || *port.Name == name {
			return *port.Port, true
		}
	}
	return 0, false
}

func endpointNodeID(ref *corev1.ObjectReference) string {
	if ref == nil {
		return ""
	}
	return strings.TrimSpace(ref.Name)
}