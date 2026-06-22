package aiservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeepCopyRoundtrip(t *testing.T) {
	original := &AIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ai",
			Namespace: "default",
		},
		Spec: AIServiceSpec{
			Provider: "openai",
			Format:   "openai",
			Model:    "gpt-4",
			Auth: AIServiceAuth{
				Type:   "apiKey",
				Secret: "my-secret",
				Header: "Authorization",
			},
			Timeout: "30s",
			Retry: AIRetryConfig{
				MaxRetries: 3,
				Backoff:    "exponential",
			},
		},
	}
	copied := original.DeepCopy()
	assert.Equal(t, original, copied)
	assert.NotSame(t, original, copied)
}