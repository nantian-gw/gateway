package main

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	gatewayfeatures "sigs.k8s.io/gateway-api/pkg/features"
)

func TestFeatureAuditFlagsDeclaredFeatureMissingUpstream(t *testing.T) {
	audit := buildFeatureAudit(
		sets.New[gatewayfeatures.FeatureName]("Gateway"),
		[]gatewayfeatures.FeatureName{"Gateway", "MadeUpFeature"},
	)

	if len(audit.StaleOrUnknown) != 1 || audit.StaleOrUnknown[0] != "MadeUpFeature" {
		t.Fatalf("StaleOrUnknown = %#v, want MadeUpFeature", audit.StaleOrUnknown)
	}
}

func TestFeatureAuditListsUnclaimedUpstreamFeatures(t *testing.T) {
	audit := buildFeatureAudit(
		sets.New[gatewayfeatures.FeatureName]("Gateway", "ListenerSet"),
		[]gatewayfeatures.FeatureName{"Gateway"},
	)

	if len(audit.UpstreamUnclaimed) != 1 || audit.UpstreamUnclaimed[0] != "ListenerSet" {
		t.Fatalf("UpstreamUnclaimed = %#v, want ListenerSet", audit.UpstreamUnclaimed)
	}
}

func TestFeatureAuditMarkdownIncludesCounts(t *testing.T) {
	audit := featureAudit{
		Declared:          []string{"Gateway"},
		UpstreamUnclaimed: []string{"ListenerSet"},
		StaleOrUnknown:    []string{"MadeUpFeature"},
	}
	var b strings.Builder
	renderAuditMarkdown(&b, audit)
	got := b.String()

	for _, want := range []string{
		"Declared features: `1`",
		"Upstream features not declared by this repository: `1`",
		"Declared features not found upstream: `1`",
		"`ListenerSet`",
		"`MadeUpFeature`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("audit markdown missing %q:\n%s", want, got)
		}
	}
}

func TestUpstreamFeatureNamesIncludeUDPRouteFeatureMap(t *testing.T) {
	if !upstreamFeatureNames().Has(gatewayfeatures.SupportUDPRoute) {
		t.Fatal("expected upstream feature audit source to include UDPRoute")
	}
}
