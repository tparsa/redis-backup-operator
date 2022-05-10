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
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupv1 "github.com/tparsa/redis-backup-operator/api/v1"
)

// RedisBackupScheduleReconciler reconciles a RedisBackupSchedule object
type RedisBackupScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func getRedisBackupScheduleCronJobSpec(rbs *backupv1.RedisBackupSchedule) *v1beta1.CronJobSpec {
	redisBackupSpec := rbs.Spec
	return &v1beta1.CronJobSpec{
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
	}
}

func getRedisBackupCronJob(rbs *backupv1.RedisBackupSchedule) *v1beta1.CronJob {
	return &v1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        rbs.Name,
			Namespace:   rbs.Namespace,
		},
		Spec: *getRedisBackupScheduleCronJobSpec(rbs),
	}
}

func getRedisBackupCronJobPatch(rbs *backupv1.RedisBackupSchedule) *v1beta1.CronJob {
	return &v1beta1.CronJob{
		Spec: *getRedisBackupScheduleCronJobSpec(rbs),
	}
}

func getRedisBackup(rbs *backupv1.RedisBackupSchedule, job *batchv1.Job) *backupv1.RedisBackup {
	return &backupv1.RedisBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbs.Name,
			Namespace: rbs.Namespace,
			Labels: map[string]string{
				"backup.yektanet.tech/redisbackupschedule": rbs.Name,
				"backup.yektanet.tech/job":                 job.Name,
			},
			Annotations: map[string]string{},
		},
		Spec: backupv1.RedisBackupSpec{
			URI:       rbs.Spec.URI,
			Image:     rbs.Spec.Image,
			RedisType: rbs.Spec.RedisType,
			Db:        rbs.Spec.Db,
		},
	}
}

//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackupschedules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackupschedules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackupschedules/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs/status,verbs=get
//+kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisBackupSchedule object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *RedisBackupScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var rbs backupv1.RedisBackupSchedule
	if err := r.Get(ctx, req.NamespacedName, &rbs); err != nil {
		log.Error(err, "unable to fetch RedisBackup")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var childCronJob v1beta1.CronJob
	if err := r.Get(ctx, req.NamespacedName, &childCronJob); err != nil {
		log.Info(fmt.Sprintf("%s %s CronJob not found. creating CronJob ...", err, rbs.Name))

		childCronJob = *getRedisBackupCronJob(&rbs)

		if err := ctrl.SetControllerReference(&rbs, &childCronJob, r.Scheme); err != nil {
			log.Error(err, "unable to set controller reference on CronJob")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, &childCronJob); err != nil {
			log.Error(err, "unable to create CronJob for RedisBackup", "cronjob", childCronJob)
			return ctrl.Result{}, err
		}
	} else {
		patch, _ := json.Marshal(*getRedisBackupCronJobPatch(&rbs))
		if err := r.Patch(ctx, &childCronJob, client.RawPatch(types.MergePatchType, patch)); err != nil {
			log.Error(err, "Couldn't patch CronJob for RedisBackup", "cronjob", childCronJob)
			return ctrl.Result{}, err
		}
	}

	var jobs batchv1.JobList
	if err := r.List(ctx, &jobs, client.InNamespace(req.Namespace), client.MatchingFields{".metadata.controller": req.Name}); err != nil {
		log.Error(err, "unable to list child Jobs")
		return ctrl.Result{}, err
	}

	for _, job := range jobs.Items {
		var rb backupv1.RedisBackup
		if err := r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, &rb); err != nil {
			log.Info(fmt.Sprintf("%s %s RedisBackup not found. creating RedisBackup ...", err, rbs.Name))

			rb = *getRedisBackup(&rbs, &job)
			if err := r.Create(ctx, &rb); err != nil {
				log.Error(err, "unable to create RedisBackup for RedisBackupSchedule", "RedisBackup", rb)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisBackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, ".metadata.controller", func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}

		if owner.Kind != "CronJob" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.RedisBackupSchedule{}).
		Complete(r)
}
