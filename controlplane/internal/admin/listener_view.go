package admin

import (
	"strings"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

const listenerDisplayAddressesMetadataKey = "nantian.dev/display-addresses"

func displayListener(listener ir.Listener) ir.Listener {
	displayAddresses := metadataAddresses(listener.Metadata[listenerDisplayAddressesMetadataKey])
	if len(displayAddresses) == 0 {
		return listener
	}

	out := listener
	out.Address = displayAddresses[0]
	out.Addresses = append([]string(nil), displayAddresses...)
	return out
}

func displayListeners(listeners []ir.Listener) []ir.Listener {
	if len(listeners) == 0 {
		return listeners
	}

	out := make([]ir.Listener, 0, len(listeners))
	for _, listener := range listeners {
		out = append(out, displayListener(listener))
	}
	return out
}

func metadataAddresses(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
