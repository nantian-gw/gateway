package admin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type surfaceContractDocument struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Surfaces      []documentedAdminSurface `json:"surfaces"`
}

type documentedAdminSurface struct {
	Name          string          `json:"name"`
	DisplayName   string          `json:"displayName"`
	BasePath      string          `json:"basePath"`
	DefaultAuth   string          `json:"defaultAuth"`
	Stability     string          `json:"stability"`
	VersionPolicy string          `json:"versionPolicy"`
	Endpoints     []routeContract `json:"endpoints"`
}

func TestControlplaneAdminRouteContractMatchesMachineReadableSurfaceDoc(t *testing.T) {
	t.Parallel()

	document := loadSurfaceContractDocument(t)
	got := surfaceEndpointsByName(t, document, "controlplane-admin")
	want := adminRouteContracts()

	if document.SchemaVersion != 1 {
		t.Fatalf("unexpected surface doc schema version: %d", document.SchemaVersion)
	}
	if !slices.Equal(canonicalizeDocumentedEndpoints(got), canonicalizeDocumentedEndpoints(want)) {
		t.Fatalf("controlplane admin route contract mismatch\nwant=%+v\ngot=%+v", want, got)
	}
}

func TestAdminSurfaceContractDocumentsVersioningMetadata(t *testing.T) {
	t.Parallel()

	document := loadSurfaceContractDocument(t)
	if document.SchemaVersion != 1 {
		t.Fatalf("unexpected surface doc schema version: %d", document.SchemaVersion)
	}

	seen := map[string]struct{}{}
	for _, surface := range document.Surfaces {
		if surface.Name == "" {
			t.Fatal("surface name must be documented")
		}
		if _, ok := seen[surface.Name]; ok {
			t.Fatalf("duplicate surface name %q", surface.Name)
		}
		seen[surface.Name] = struct{}{}

		if surface.DisplayName == "" {
			t.Fatalf("surface %q must document displayName", surface.Name)
		}
		if surface.BasePath == "" {
			t.Fatalf("surface %q must document basePath", surface.Name)
		}
		if surface.DefaultAuth != "bearer-when-configured" {
			t.Fatalf("surface %q has unexpected defaultAuth %q", surface.Name, surface.DefaultAuth)
		}
		if surface.Stability != "stable-v1" {
			t.Fatalf("surface %q must declare stability=stable-v1, got %q", surface.Name, surface.Stability)
		}
		if surface.VersionPolicy != "additive-compatible" {
			t.Fatalf("surface %q must declare versionPolicy=additive-compatible, got %q", surface.Name, surface.VersionPolicy)
		}
	}
}

func loadSurfaceContractDocument(t *testing.T) surfaceContractDocument {
	t.Helper()

	path := filepath.Join("..", "..", "..", "docs", "contracts", "admin-api-surface.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read surface contract doc: %v", err)
	}

	var document surfaceContractDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode surface contract doc: %v", err)
	}
	return document
}

func surfaceEndpointsByName(t *testing.T, document surfaceContractDocument, name string) []routeContract {
	t.Helper()

	for _, surface := range document.Surfaces {
		if surface.Name == name {
			return surface.Endpoints
		}
	}

	t.Fatalf("surface %q not found in machine-readable contract", name)
	return nil
}

func canonicalizeDocumentedEndpoints(endpoints []routeContract) []routeContract {
	cloned := append([]routeContract(nil), endpoints...)
	slices.SortFunc(cloned, func(a, b routeContract) int {
		if a.Path != b.Path {
			if a.Path < b.Path {
				return -1
			}
			return 1
		}
		if a.Method != b.Method {
			if a.Method < b.Method {
				return -1
			}
			return 1
		}
		if a.Auth != b.Auth {
			if a.Auth < b.Auth {
				return -1
			}
			return 1
		}
		if a.ContentType < b.ContentType {
			return -1
		}
		if a.ContentType > b.ContentType {
			return 1
		}
		return 0
	})
	return cloned
}
