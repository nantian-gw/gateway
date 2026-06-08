package gatewayapi

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetListenerSetV1(ctx context.Context, c client.Client, key client.ObjectKey) (*gatewayv1.ListenerSet, *gatewayv1.ListenerSet, error) {
	var current gatewayv1.ListenerSet
	if err := c.Get(ctx, key, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return &current, &current, nil
}

func UpdateListenerSetV1Status(ctx context.Context, c client.Client, current *gatewayv1.ListenerSet, desired gatewayv1.ListenerSetStatus) error {
	current.Status = desired
	return c.Status().Update(ctx, current)
}