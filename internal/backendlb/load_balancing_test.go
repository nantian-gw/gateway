package backendlb

import (
	"strings"
	"testing"

	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
)

func TestValidateLoadBalancing(t *testing.T) {
	roundRobin := backendlbv1alpha2.LoadBalancingStrategyTypeRoundRobin
	leastRequest := backendlbv1alpha2.LoadBalancingStrategyTypeLeastRequest
	random := backendlbv1alpha2.LoadBalancingStrategyTypeRandom
	consistentHash := backendlbv1alpha2.LoadBalancingStrategyTypeConsistentHash
	sourceIP := backendlbv1alpha2.HashKeyTypeSourceIP
	header := backendlbv1alpha2.HashKeyTypeHeader
	hostname := backendlbv1alpha2.HashKeyTypeHostname
	unsupportedType := backendlbv1alpha2.LoadBalancingStrategyType("Maglev")
	unsupportedKey := backendlbv1alpha2.HashKeyType("Cookie")

	tests := []struct {
		name    string
		policy  *backendlbv1alpha2.LoadBalancingPolicy
		wantErr string
	}{
		{
			name: "nil policy is valid",
		},
		{
			name:   "empty policy defaults to round robin",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{},
		},
		{
			name: "round robin rejects consistent hash config",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &roundRobin,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &sourceIP},
			},
			wantErr: "round robin strategy does not accept consistentHash config",
		},
		{
			name: "least request rejects consistent hash config",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &leastRequest,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &sourceIP},
			},
			wantErr: "least request strategy does not accept consistentHash config",
		},
		{
			name: "random rejects consistent hash config",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &random,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &sourceIP},
			},
			wantErr: "random strategy does not accept consistentHash config",
		},
		{
			name: "consistent hash requires config",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type: &consistentHash,
			},
			wantErr: "consistent hash strategy requires consistentHash config",
		},
		{
			name: "consistent hash source ip is valid without header name",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &sourceIP},
			},
		},
		{
			name: "consistent hash hostname is valid without header name",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &hostname},
			},
		},
		{
			name: "consistent hash source ip rejects header name",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type: &consistentHash,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{
					KeyType:    &sourceIP,
					HeaderName: ptr("x-session"),
				},
			},
			wantErr: "sourceip strategy does not accept headerName",
		},
		{
			name: "consistent hash header requires header name",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &header},
			},
			wantErr: "header strategy requires headerName",
		},
		{
			name: "consistent hash header accepts non-empty header name",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type: &consistentHash,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{
					KeyType:    &header,
					HeaderName: ptr("x-session"),
				},
			},
		},
		{
			name: "unsupported strategy is rejected",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type: &unsupportedType,
			},
			wantErr: "load balancing type \"Maglev\" is not supported",
		},
		{
			name: "unsupported consistent hash key is rejected",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{KeyType: &unsupportedKey},
			},
			wantErr: "consistent hash key type \"Cookie\" is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLoadBalancing(tt.policy)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateLoadBalancing() returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateLoadBalancing() returned nil error, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateLoadBalancing() error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveLoadBalancingType(t *testing.T) {
	randomWithSpaces := backendlbv1alpha2.LoadBalancingStrategyType(" Random ")

	tests := []struct {
		name   string
		policy *backendlbv1alpha2.LoadBalancingPolicy
		want   backendlbv1alpha2.LoadBalancingStrategyType
	}{
		{
			name: "nil policy returns empty type",
			want: "",
		},
		{
			name:   "empty policy defaults to round robin",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{},
			want:   backendlbv1alpha2.LoadBalancingStrategyTypeRoundRobin,
		},
		{
			name: "explicit type is trimmed",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				Type: &randomWithSpaces,
			},
			want: backendlbv1alpha2.LoadBalancingStrategyTypeRandom,
		},
		{
			name: "consistent hash config implies consistent hash strategy",
			policy: &backendlbv1alpha2.LoadBalancingPolicy{
				ConsistentHash: &backendlbv1alpha2.ConsistentHashPolicy{},
			},
			want: backendlbv1alpha2.LoadBalancingStrategyTypeConsistentHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveLoadBalancingType(tt.policy); got != tt.want {
				t.Fatalf("EffectiveLoadBalancingType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEffectiveConsistentHashKeyType(t *testing.T) {
	headerWithSpaces := backendlbv1alpha2.HashKeyType(" Header ")

	if got := EffectiveConsistentHashKeyType(nil); got != "" {
		t.Fatalf("EffectiveConsistentHashKeyType(nil) = %q, want empty", got)
	}
	if got := EffectiveConsistentHashKeyType(&backendlbv1alpha2.ConsistentHashPolicy{}); got != "" {
		t.Fatalf("EffectiveConsistentHashKeyType(empty) = %q, want empty", got)
	}
	if got := EffectiveConsistentHashKeyType(&backendlbv1alpha2.ConsistentHashPolicy{KeyType: &headerWithSpaces}); got != backendlbv1alpha2.HashKeyTypeHeader {
		t.Fatalf("EffectiveConsistentHashKeyType() = %q, want %q", got, backendlbv1alpha2.HashKeyTypeHeader)
	}
}
