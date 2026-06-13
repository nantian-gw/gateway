package admin

import (
	"reflect"
	"testing"
)

func TestEmptyPrometheusResponse(t *testing.T) {
	got := emptyPrometheusResponse()
	want := map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result":     []any{},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("emptyPrometheusResponse() = %#v, want %#v", got, want)
	}
}
