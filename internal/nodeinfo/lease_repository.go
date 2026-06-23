package nodeinfo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
)

const (
	defaultLeaseNamespace      = "nantian-gw"
	defaultLeasePrefix         = "nantian-gw-node"
	defaultLeaseDuration       = 300 * time.Second
	managedByLabelKey          = "app.kubernetes.io/managed-by"
	managedByLabelValue        = "nantian-gw"
	componentLabelKey          = "nantian.dev/component"
	componentLabelValue        = "node-status"
	nodeIDAnnotationKey        = "nantian.dev/node-id"
	nodeStatusAnnotationKey    = "nantian.dev/node-status"
	serviceAccountNamespaceRef = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

type LeaseRepository struct {
	reader    client.Reader
	writer    client.Writer
	namespace string
	prefix    string
	logger    *slog.Logger
}

func NewLeaseRepository(
	reader client.Reader,
	writer client.Writer,
	namespace, prefix string,
	logger *slog.Logger,
) *LeaseRepository {
	if logger == nil {
		logger = slog.Default()
	}

	return &LeaseRepository{
		reader:    reader,
		writer:    writer,
		namespace: ResolveNamespace(namespace),
		prefix:    sanitizeLeasePrefix(prefix),
		logger:    logger,
	}
}

func ResolveNamespace(explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("POD_NAMESPACE")); value != "" {
		return value
	}
	raw, err := os.ReadFile(serviceAccountNamespaceRef)
	if err == nil {
		if value := strings.TrimSpace(string(raw)); value != "" {
			return value
		}
	}

	return defaultLeaseNamespace
}

func (r *LeaseRepository) Namespace() string {
	return r.namespace
}

func (r *LeaseRepository) Prefix() string {
	return r.prefix
}

func (r *LeaseRepository) Get(ctx context.Context, nodeID string) (ir.NodeStatus, bool, error) {
	var lease coordinationv1.Lease
	err := r.reader.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      r.leaseName(nodeID),
	}, &lease)
	if apierrors.IsNotFound(err) {
		return ir.NodeStatus{}, false, nil
	}
	if err != nil {
		return ir.NodeStatus{}, false, err
	}

	status, err := parseLease(&lease)
	if err != nil {
		r.logger.Warn("failed to decode node status lease", "lease", lease.Name, "error", err)
		return ir.NodeStatus{}, false, nil
	}

	return status, true, nil
}

func (r *LeaseRepository) List(ctx context.Context) ([]ir.NodeStatus, error) {
	var leases coordinationv1.LeaseList
	if err := r.reader.List(
		ctx,
		&leases,
		client.InNamespace(r.namespace),
		client.MatchingLabels{
			managedByLabelKey: managedByLabelValue,
			componentLabelKey: componentLabelValue,
		},
	); err != nil {
		return nil, err
	}

	out := make([]ir.NodeStatus, 0, len(leases.Items))
	for i := range leases.Items {
		status, err := parseLease(&leases.Items[i])
		if err != nil {
			r.logger.Warn("failed to decode node status lease", "lease", leases.Items[i].Name, "error", err)
			continue
		}
		out = append(out, status)
	}

	return out, nil
}

func (r *LeaseRepository) Upsert(ctx context.Context, status ir.NodeStatus) error {
	if status.NodeID == "" {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var lease coordinationv1.Lease
		key := client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.leaseName(status.NodeID),
		}
		err := r.reader.Get(ctx, key, &lease)
		switch {
		case apierrors.IsNotFound(err):
			lease = coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
			}
			if err := applyStatusToLease(&lease, status); err != nil {
				return err
			}
			return r.writer.Create(ctx, &lease)
		case err != nil:
			return err
		default:
			if err := applyStatusToLease(&lease, status); err != nil {
				return err
			}
			return r.writer.Update(ctx, &lease)
		}
	})
}

func applyStatusToLease(lease *coordinationv1.Lease, status ir.NodeStatus) error {
	payload, err := json.Marshal(clone(status))
	if err != nil {
		return fmt.Errorf("marshal node status: %w", err)
	}

	if lease.Labels == nil {
		lease.Labels = make(map[string]string, 2)
	}
	lease.Labels[managedByLabelKey] = managedByLabelValue
	lease.Labels[componentLabelKey] = componentLabelValue

	if lease.Annotations == nil {
		lease.Annotations = make(map[string]string, 2)
	}
	lease.Annotations[nodeIDAnnotationKey] = status.NodeID
	lease.Annotations[nodeStatusAnnotationKey] = string(payload)

	leaseDurationSeconds := int32(defaultLeaseDuration / time.Second)
	holderIdentity := status.NodeID
	renewTime := metav1.NewMicroTime(lastSeen(status))

	lease.Spec.HolderIdentity = &holderIdentity
	lease.Spec.LeaseDurationSeconds = &leaseDurationSeconds
	lease.Spec.RenewTime = &renewTime
	if !status.ConnectedAt.IsZero() {
		acquireTime := metav1.NewMicroTime(status.ConnectedAt.UTC())
		lease.Spec.AcquireTime = &acquireTime
	}

	return nil
}

func parseLease(lease *coordinationv1.Lease) (ir.NodeStatus, error) {
	raw := strings.TrimSpace(lease.Annotations[nodeStatusAnnotationKey])
	if raw == "" {
		return ir.NodeStatus{}, fmt.Errorf("missing %s annotation", nodeStatusAnnotationKey)
	}

	var status ir.NodeStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return ir.NodeStatus{}, fmt.Errorf("unmarshal node status: %w", err)
	}

	if status.NodeID == "" {
		status.NodeID = strings.TrimSpace(lease.Annotations[nodeIDAnnotationKey])
	}
	if status.NodeID == "" && lease.Spec.HolderIdentity != nil {
		status.NodeID = strings.TrimSpace(*lease.Spec.HolderIdentity)
	}
	if status.LastSeenAt.IsZero() && lease.Spec.RenewTime != nil {
		status.LastSeenAt = lease.Spec.RenewTime.Time.UTC()
	}
	if status.ConnectedAt.IsZero() && lease.Spec.AcquireTime != nil {
		status.ConnectedAt = lease.Spec.AcquireTime.Time.UTC()
	}
	if !status.ConnectedAt.IsZero() {
		status.ConnectedAt = status.ConnectedAt.UTC()
	}
	if !status.DisconnectedAt.IsZero() {
		status.DisconnectedAt = status.DisconnectedAt.UTC()
	}
	if !status.LastSeenAt.IsZero() {
		status.LastSeenAt = status.LastSeenAt.UTC()
	}

	return clone(status), nil
}

func sanitizeLeasePrefix(raw string) string {
	const maxPrefixLength = 46

	var builder strings.Builder
	previousDash := true
	for _, value := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case value >= 'a' && value <= 'z':
			builder.WriteRune(value)
			previousDash = false
		case value >= '0' && value <= '9':
			builder.WriteRune(value)
			previousDash = false
		case !previousDash:
			builder.WriteByte('-')
			previousDash = true
		}
	}

	prefix := strings.Trim(builder.String(), "-")
	if prefix == "" {
		prefix = defaultLeasePrefix
	}
	if len(prefix) > maxPrefixLength {
		prefix = strings.Trim(prefix[:maxPrefixLength], "-")
	}
	if prefix == "" {
		prefix = defaultLeasePrefix
	}

	return prefix
}

func (r *LeaseRepository) leaseName(nodeID string) string {
	sum := sha256.Sum256([]byte(nodeID))
	return fmt.Sprintf("%s-%x", r.prefix, sum[:8])
}

func lastSeen(status ir.NodeStatus) time.Time {
	if !status.LastSeenAt.IsZero() {
		return status.LastSeenAt.UTC()
	}

	return time.Now().UTC()
}
