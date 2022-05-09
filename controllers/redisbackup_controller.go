/*
Copyright 2022.

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
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupv1 "github.com/tparsa/redis-backup-operator/api/v1"
)

// RedisBackupReconciler reconciles a RedisBackup object
type RedisBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *RedisBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var redisBackup backupv1.RedisBackup
	if err := r.Get(ctx, req.NamespacedName, &redisBackup); err != nil {
		log.Error(err, "unable to fetch RedisBackup")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var childCronJob v1beta1.CronJob
	if err := r.Get(ctx, req.NamespacedName, &childCronJob); err != nil {
		name := redisBackup.Name
		log.Info(fmt.Sprintf("%s CronJob not found. creating CronJob ...", name))
		redisBackupSpec := redisBackup.Spec
		childCronJob = v1beta1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        name,
				Namespace:   redisBackup.Namespace,
			},
			Spec: v1beta1.CronJobSpec{
				Schedule:          redisBackupSpec.Schedule,
				ConcurrencyPolicy: v1beta1.ForbidConcurrent,
				JobTemplate: v1beta1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: "Never",
								Containers: []corev1.Container{
									{
										Name:  "backup",
										Image: redisBackupSpec.Image,
										Args: []string{
											fmt.Sprintf("--uri=%s", redisBackupSpec.URI),
											fmt.Sprintf("--type=%s", redisBackupSpec.RedisType),
											"--output-stdout",
											"--mode=dump",
											fmt.Sprintf("--db=%d", redisBackupSpec.Db),
										},
										ImagePullPolicy: "Always",
									},
								},
							},
						},
					},
				},
			},
		}
		if err := r.Create(ctx, &childCronJob); err != nil {
			log.Error(err, "unable to create CronJob for RedisBackup", "cronjob", childCronJob)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.RedisBackup{}).
		Complete(r)
}
