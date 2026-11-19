package controller

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"reflect"
	"strings"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubernetesAuditSink stores immutable audit events as StationAuditEvent resources.
type KubernetesAuditSink struct {
	client client.Client
	reader client.Reader
}

func NewKubernetesAuditSink(writer client.Client, reader client.Reader) *KubernetesAuditSink {
	return &KubernetesAuditSink{client: writer, reader: reader}
}

func (s *KubernetesAuditSink) Record(ctx context.Context, event RebindAuditEvent) error {
	if event.Namespace == "" || event.OperationUID == "" || event.OperationName == "" || event.ClusterID == "" || event.StationUID == "" || event.StationName == "" || event.Actor == "" || event.Outcome == "" || event.ResultReason == "" || event.Reason == "" {
		return errors.New("incomplete Station audit event")
	}
	resource := &stationv1alpha1.StationAuditEvent{
		ObjectMeta: metav1.ObjectMeta{Namespace: event.Namespace, Name: AuditEventName(event.OperationUID)},
		Spec: stationv1alpha1.StationAuditEventSpec{
			Operation:              stationv1alpha1.StationReference{Name: event.OperationName, UID: event.OperationUID},
			OperationID:            event.OperationUID,
			ClusterID:              event.ClusterID,
			Action:                 "Rebind",
			Outcome:                event.Outcome,
			ResultReason:           event.ResultReason,
			Station:                stationv1alpha1.StationReference{Name: event.StationName, UID: event.StationUID},
			ObservedStationUID:     event.ObservedStationUID,
			Actor:                  event.Actor,
			OccurredAt:             metav1.NewTime(event.OccurredAt.UTC()),
			PreviousNodeUID:        event.OldNodeUID,
			TargetNodeUID:          event.NewNodeUID,
			PreviousBindingVersion: event.OldVersion,
			ResultBindingVersion:   event.NewVersion,
			Reason:                 event.Reason,
		},
	}
	if err := s.client.Create(ctx, resource); err == nil {
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create StationAuditEvent: %w", err)
	}
	existing := &stationv1alpha1.StationAuditEvent{}
	key := types.NamespacedName{Namespace: event.Namespace, Name: resource.Name}
	if err := s.reader.Get(ctx, key, existing); err != nil {
		return fmt.Errorf("read StationAuditEvent after create race: %w", err)
	}
	// The event timestamp is assigned by the first successful writer; retries reuse it.
	resource.Spec.OccurredAt = existing.Spec.OccurredAt
	if !reflect.DeepEqual(existing.Spec, resource.Spec) {
		return errors.New("StationAuditEvent name is already owned by different event data")
	}
	return nil
}

func AuditEventName(operationUID string) string {
	digest := sha256.Sum256([]byte(operationUID))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:16])
	return "audit-" + strings.ToLower(encoded)
}
