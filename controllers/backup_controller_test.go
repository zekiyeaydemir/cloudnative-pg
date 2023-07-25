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

package controllers

import (
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	schemeBuilder "github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("backup reconciler unit tests", func() {
	It("should do nothing if there is nothing", func(ctx SpecContext) {
		clusterNamespace := "cluster-test"
		clusterName := "myTestCluster"
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
			Spec: apiv1.ClusterSpec{
				Backup: &apiv1.BackupConfiguration{},
			},
		}
		backupName := "theBackup"
		backup := &apiv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      backupName,
			},
			Spec: apiv1.BackupSpec{
				Cluster: apiv1.LocalObjectReference{
					Name: clusterName,
				},
			},
			Status: apiv1.BackupStatus{
				Phase: apiv1.BackupPhasePending,
			},
		}
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
			},
		}
		//var childPods corev1.PodList
		//if err := r.List(ctx, &childPods,
		//	client.InNamespace(cluster.Namespace),
		//	client.MatchingFields{podOwnerKey: cluster.Name},
		//); err != nil {
		//	log.FromContext(ctx).Error(err, "Unable to list child pods resource")
		//	return corev1.PodList{}, err
		//}

		client := fake.NewClientBuilder().WithScheme(schemeBuilder.BuildWithAllKnownScheme()).
			WithObjects(cluster, backup, pod).Build()
		eventRecorder := record.NewFakeRecorder(5)
		backupReconciler := &BackupReconciler{
			Client:               client,
			Scheme:               schemeBuilder.BuildWithAllKnownScheme(),
			Recorder:             eventRecorder,
			instanceStatusClient: newInstanceStatusClient(),
		}

		var stuff apiv1.Backup
		err := backupReconciler.Get(ctx, types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      backupName,
		}, &stuff)
		Expect(err).ToNot(HaveOccurred(), "Y")

		var moStuff apiv1.Backup
		err = client.Get(ctx, types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      backupName,
		}, &moStuff)
		Expect(err).ToNot(HaveOccurred(), "X")

		res, err := backupReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: clusterNamespace,
				Name:      backupName,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
	})

	It("should do nothing if the backup is non-pending", func(ctx SpecContext) {
		clusterNamespace := "cluster-test"
		clusterName := "myTestCluster"
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
		}
		backupName := "theBackup"
		backup := &apiv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      backupName,
			},
			Spec: apiv1.BackupSpec{
				Cluster: apiv1.LocalObjectReference{
					Name: "badClusterName",
				},
			},
			Status: apiv1.BackupStatus{
				Phase: apiv1.BackupPhaseFailed,
			},
		}
		client := fake.NewClientBuilder().WithScheme(schemeBuilder.BuildWithAllKnownScheme()).
			WithObjects(backup, cluster).Build()
		eventRecorder := record.NewFakeRecorder(5)
		backupReconciler := &BackupReconciler{
			Client:               client,
			Scheme:               schemeBuilder.BuildWithAllKnownScheme(),
			Recorder:             eventRecorder,
			instanceStatusClient: newInstanceStatusClient(),
		}

		var moStuff apiv1.Backup
		err := client.Get(ctx, types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      backupName,
		}, &moStuff)
		Expect(err).ToNot(HaveOccurred(), "X")

		res, err := backupReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: clusterNamespace,
				Name:      backupName,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
	})
})
