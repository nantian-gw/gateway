package infrastructure

import "testing"

func TestDefaultOptionsUseCanonicalInstallNames(t *testing.T) {
	opts := DefaultOptions()

	if opts.SharedServiceName != "nantian-gw-dataplane" {
		t.Fatalf("SharedServiceName = %q, want nantian-gw-dataplane", opts.SharedServiceName)
	}
	if got := opts.DataplaneSelector["app"]; got != "nantian-gw-dataplane" {
		t.Fatalf("DataplaneSelector[app] = %q, want nantian-gw-dataplane", got)
	}
	if defaultDataplaneNetworkPolicyName != "nantian-gw-dataplane" {
		t.Fatalf("defaultDataplaneNetworkPolicyName = %q, want nantian-gw-dataplane", defaultDataplaneNetworkPolicyName)
	}
}
