package aiservicev1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "gateway.nantian.dev", Version: "v1alpha1"}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type AIService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AIServiceSpec   `json:"spec,omitempty"`
	Status            AIServiceStatus `json:"status,omitempty"`
}

type AIServiceSpec struct {
	Provider      string                 `json:"provider"`
	Format        string                 `json:"format,omitempty"`
	Model         string                 `json:"model"`
	Auth          AIServiceAuth          `json:"auth,omitempty"`
	Timeout       string                 `json:"timeout,omitempty"`
	Retry         AIRetryConfig          `json:"retry,omitempty"`
	Observability AIObservabilityConfig  `json:"observability,omitempty"`
}

type AIServiceAuth struct {
	Type   string `json:"type,omitempty"`
	Secret string `json:"secret,omitempty"`
	Key    string `json:"key,omitempty"`
	Header string `json:"header,omitempty"`
}

type AIRetryConfig struct {
	MaxRetries uint32 `json:"maxRetries,omitempty"`
	Backoff    string `json:"backoff,omitempty"`
}

type AIObservabilityConfig struct {
	Langfuse LangfuseConfig `json:"langfuse,omitempty"`
	OTel     OTelConfig     `json:"otel,omitempty"`
}

type LangfuseConfig struct {
	Host      string `json:"host,omitempty"`
	PublicKey string `json:"publicKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

type OTelConfig struct {
	Endpoint    string `json:"endpoint,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

type AIServiceStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type AIServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIService `json:"items"`
}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &AIService{}, &AIServiceList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *AIService) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *AIService) DeepCopy() *AIService {
	if in == nil {
		return nil
	}
	var out AIService
	mustRoundTrip(in, &out)
	return &out
}

func (in *AIServiceList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *AIServiceList) DeepCopy() *AIServiceList {
	if in == nil {
		return nil
	}
	var out AIServiceList
	mustRoundTrip(in, &out)
	return &out
}

func mustRoundTrip(in any, out any) {
	data, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		panic(err)
	}
}