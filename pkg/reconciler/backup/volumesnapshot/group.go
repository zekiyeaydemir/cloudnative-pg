/*
Copyright The CloudNativePG Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package volumesnapshot

import (
	"context"
	"fmt"

	storagegroupsnapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
	storagesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/log"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// createVolumeGroupSnapshot creates a volume group snapshot for a given cluster
func (se *Reconciler) createVolumeGroupSnapshot(
	ctx context.Context,
	cluster *apiv1.Cluster,
	backup *apiv1.Backup,
	targetPod *corev1.Pod,
) error {
	var snapshotClassName *string
	if len(cluster.Spec.Backup.VolumeGroupSnapshot.ClassName) > 0 {
		snapshotClassName = &cluster.Spec.Backup.VolumeGroupSnapshot.ClassName
	}

	snapshot := storagegroupsnapshotv1alpha1.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
		Spec: storagegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
			Source: storagegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						utils.InstanceNameLabelName: targetPod.Name,
					},
				},
			},
			VolumeGroupSnapshotClassName: snapshotClassName,
		},
	}
	if snapshot.Labels == nil {
		snapshot.Labels = map[string]string{}
	}
	if snapshot.Annotations == nil {
		snapshot.Annotations = map[string]string{}
	}
	if err := se.enrichSnapshot(ctx, &snapshot.ObjectMeta, backup, cluster, targetPod); err != nil {
		return err
	}

	if err := se.cli.Create(ctx, &snapshot); err != nil {
		if !apierrs.IsAlreadyExists(err) {
			return fmt.Errorf("while creating VolumeGroupSnapshot %s: %w", snapshot.Name, err)
		}

		return se.enrichVolumeGroupSnapshot(ctx, cluster, backup)
	}

	return nil
}

// enrichVolumeGroupSnapshot enriches the VolumeSnapshots resources
// created by the VolumeGroupSnapshot object with all the required
// metadata
func (se *Reconciler) enrichVolumeGroupSnapshot(
	ctx context.Context,
	cluster *apiv1.Cluster,
	backup *apiv1.Backup,
) error {
	contextLogger := log.FromContext(ctx)

	var groupSnapshot storagegroupsnapshotv1alpha1.VolumeGroupSnapshot
	if err := se.cli.Get(
		ctx,
		client.ObjectKey{Namespace: backup.Namespace, Name: backup.Name},
		&groupSnapshot,
	); err != nil {
		if apierrs.IsNotFound(err) {
			return nil
		}

		return err
	}

	// Wait for the CSI driver to have created the independent volume snapshots
	if len(groupSnapshot.Status.VolumeSnapshotRefList) == 0 {
		return nil
	}

	// The volume group snapshot is still not bound
	if groupSnapshot.Status.BoundVolumeGroupSnapshotContentName == nil ||
		len(*groupSnapshot.Status.BoundVolumeGroupSnapshotContentName) == 0 {
		return nil
	}

	// Get the bound snapshot content
	var groupSnapshotContent storagegroupsnapshotv1alpha1.VolumeGroupSnapshotContent
	if err := se.cli.Get(
		ctx,
		client.ObjectKey{Name: *groupSnapshot.Status.BoundVolumeGroupSnapshotContentName},
		&groupSnapshotContent,
	); err != nil {
		if apierrs.IsNotFound(err) {
			return nil
		}

		return err
	}

	// Wait for the volume group snapshot controller to bind all the volumes
	if len(groupSnapshotContent.Spec.Source.PersistentVolumeNames) != len(groupSnapshot.Status.VolumeSnapshotRefList) {
		return nil
	}

	// Enrich the volume snapshots
	for i := range groupSnapshot.Status.VolumeSnapshotRefList {
		snapshotRef := groupSnapshot.Status.VolumeSnapshotRefList[i]
		pvName := groupSnapshotContent.Spec.Source.PersistentVolumeNames[i]

		var pv corev1.PersistentVolume
		if err := se.cli.Get(
			ctx,
			client.ObjectKey{Name: pvName},
			&pv,
		); err != nil {
			return err
		}

		if pv.Spec.ClaimRef == nil {
			contextLogger.Info("Unbound PV claimRef for a Snapshotter PV",
				"snapshotRef", snapshotRef, "pvName", pvName)
			continue
		}

		if err := se.enrichVolumeGroupSnapshotMember(
			ctx,
			cluster,
			backup,
			&groupSnapshot,
			snapshotRef,
			pv.Spec.ClaimRef.Name,
		); err != nil {
			return err
		}
	}

	return nil
}

// enrichVolumeSnapshot enriches a Volume Snapshot created by a VolumeGroupSnapshot
func (se *Reconciler) enrichVolumeGroupSnapshotMember(
	ctx context.Context,
	cluster *apiv1.Cluster,
	backup *apiv1.Backup,
	groupSnapshot *storagegroupsnapshotv1alpha1.VolumeGroupSnapshot,
	snapshotRef corev1.ObjectReference,
	pvcName string,
) error {
	var snapshot storagesnapshotv1.VolumeSnapshot
	var pvc corev1.PersistentVolumeClaim

	if err := se.cli.Get(
		ctx,
		client.ObjectKey{
			Namespace: snapshotRef.Namespace,
			Name:      snapshotRef.Name,
		},
		&snapshot,
	); err != nil {
		if apierrs.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := se.cli.Get(
		ctx,
		client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      pvcName,
		},
		&pvc,
	); err != nil {
		if apierrs.IsNotFound(err) {
			return nil
		}
		return err
	}

	snapshotConfig := backup.GetVolumeSnapshotCommonConfiguration(cluster)

	if snapshot.Labels == nil {
		snapshot.Labels = make(map[string]string)
	}
	if snapshot.Annotations == nil {
		snapshot.Annotations = make(map[string]string)
	}

	origSnapshot := snapshot.DeepCopy()

	utils.MergeMap(snapshot.Labels, groupSnapshot.Labels)
	utils.MergeMap(snapshot.Labels, pvc.Labels)
	utils.MergeMap(snapshot.Labels, snapshotConfig.Labels)
	utils.MergeMap(snapshot.Annotations, groupSnapshot.Annotations)
	utils.MergeMap(snapshot.Annotations, pvc.Annotations)
	utils.MergeMap(snapshot.Annotations, snapshotConfig.Annotations)
	transferLabelsToAnnotations(snapshot.Labels, snapshot.Annotations)

	return se.cli.Patch(ctx, &snapshot, client.MergeFrom(origSnapshot))
}
