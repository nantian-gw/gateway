package lbpolicy

import (
	"strings"
	"testing"

	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
)

func TestValidateLoadBalancing(t *testing.T) {
	roundRobin := backendlb.LoadBalancingStrategyTypeRoundRobin
	leastRequest := backendlb.LoadBalancingStrategyTypeLeastRequest
	random := backendlb.LoadBalancingStrategyTypeRandom
	consistentHash := backendlb.LoadBalancingStrategyTypeConsistentHash
	sourceIP := backendlb.HashKeyTypeSourceIP
	header := backendlb.HashKeyTypeHeader
	hostname := backendlb.HashKeyTypeHostname
	unsupportedType := backendlb.LoadBalancingStrategyType("Maglev")
	unsupportedKey := backendlb.HashKeyType("Cookie")

	tests := []struct {
		name    string
		policy  *backendlb.LoadBalancingPolicy
		wantErr string
	}{
		{
			name: "nil policy is valid",
		},
		{
			name:   "empty policy defaults to round robin",
			policy: &backendlb.LoadBalancingPolicy{},
		},
		{
			name: "round robin rejects consistent hash config",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &roundRobin,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &sourceIP},
			},
			wantErr: "round robin strategy does not accept consistentHash config",
		},
		{
			name: "least request rejects consistent hash config",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &leastRequest,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &sourceIP},
			},
			wantErr: "least request strategy does not accept consistentHash config",
		},
		{
			name: "random rejects consistent hash config",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &random,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &sourceIP},
			},
			wantErr: "random strategy does not accept consistentHash config",
		},
		{
			name: "consistent hash requires config",
			policy: &backendlb.LoadBalancingPolicy{
				Type: &consistentHash,
			},
			wantErr: "consistent hash strategy requires consistentHash config",
		},
		{
			name: "consistent hash source ip is valid without header name",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &sourceIP},
			},
		},
		{
			name: "consistent hash hostname is valid without header name",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &hostname},
			},
		},
		{
			name: "consistent hash source ip rejects header name",
			policy: &backendlb.LoadBalancingPolicy{
				Type: &consistentHash,
				ConsistentHash: &backendlb.ConsistentHashPolicy{
					KeyType:    &sourceIP,
					HeaderName: ptr("x-session"),
				},
			},
			wantErr: "sourceip strategy does not accept headerName",
		},
		{
			name: "consistent hash header requires header name",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &header},
			},
			wantErr: "header strategy requires headerName",
		},
		{
			name: "consistent hash header accepts non-empty header name",
			policy: &backendlb.LoadBalancingPolicy{
				Type: &consistentHash,
				ConsistentHash: &backendlb.ConsistentHashPolicy{
					KeyType:    &header,
					HeaderName: ptr("x-session"),
				},
			},
		},
		{
			name: "unsupported strategy is rejected",
			policy: &backendlb.LoadBalancingPolicy{
				Type: &unsupportedType,
			},
			wantErr: "load balancing type \"Maglev\" is not supported",
		},
		{
			name: "unsupported consistent hash key is rejected",
			policy: &backendlb.LoadBalancingPolicy{
				Type:           &consistentHash,
				ConsistentHash: &backendlb.ConsistentHashPolicy{KeyType: &unsupportedKey},
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
	randomWithSpaces := backendlb.LoadBalancingStrategyType(" Random ")

	tests := []struct {
		name   string
		policy *backendlb.LoadBalancingPolicy
		want   backendlb.LoadBalancingStrategyType
	}{
		{
			name: "nil policy returns empty type",
			want: "",
		},
		{
			name:   "empty policy defaults to round robin",
			policy: &backendlb.LoadBalancingPolicy{},
			want:   backendlb.LoadBalancingStrategyTypeRoundRobin,
		},
		{
			name: "explicit type is trimmed",
			policy: &backendlb.LoadBalancingPolicy{
				Type: &randomWithSpaces,
			},
			want: backendlb.LoadBalancingStrategyTypeRandom,
		},
		{
			name: "consistent hash config implies consistent hash strategy",
			policy: &backendlb.LoadBalancingPolicy{
				ConsistentHash: &backendlb.ConsistentHashPolicy{},
			},
			want: backendlb.LoadBalancingStrategyTypeConsistentHash,
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
	headerWithSpaces := backendlb.HashKeyType(" Header ")

	if got := EffectiveConsistentHashKeyType(nil); got != "" {
		t.Fatalf("EffectiveConsistentHashKeyType(nil) = %q, want empty", got)
	}
	if got := EffectiveConsistentHashKeyType(&backendlb.ConsistentHashPolicy{}); got != "" {
		t.Fatalf("EffectiveConsistentHashKeyType(empty) = %q, want empty", got)
	}
	if got := EffectiveConsistentHashKeyType(&backendlb.ConsistentHashPolicy{KeyType: &headerWithSpaces}); got != backendlb.HashKeyTypeHeader {
		t.Fatalf("EffectiveConsistentHashKeyType() = %q, want %q", got, backendlb.HashKeyTypeHeader)
	}
}
