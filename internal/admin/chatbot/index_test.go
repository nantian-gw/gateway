package chatbot

import "testing"

func TestResourceRefString(t *testing.T) {
	ref := ResourceRef{Kind: kindGateway, Namespace: "default", Name: "public"}
	if got := ref.String(); got != "Gateway default/public" {
		t.Errorf("ResourceRef.String() = %q, want %q", got, "Gateway default/public")
	}
}
