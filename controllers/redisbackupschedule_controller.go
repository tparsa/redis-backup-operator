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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	rs := redisBackupSpec.RetentionSpec
	keepLast := rs.KeepLast
	if keepLast == 0 {
		keepLast = 1
	}
	retentionArgs := []string{
		fmt.Sprintf("--keep-last=%d", keepLast),
		fmt.Sprintf("--keep-hourly=%d", rs.KeepHourly),
		fmt.Sprintf("--keep-daily=%d", rs.KeepDaily),
		fmt.Sprintf("--keep-weekly=%d", rs.KeepWeekly),
		fmt.Sprintf("--keep-monthly=%d", rs.KeepMonthly),
		fmt.Sprintf("--keep-yearly=%d", rs.KeepYearly),
	}

	args := []string{
		fmt.Sprintf("--name=%s-%s", rbs.Namespace, rbs.Name),
		fmt.Sprintf("--type=%s", redisBackupSpec.RedisType),
		fmt.Sprintf("--bucket=%s", redisBackupSpec.Bucket),
		fmt.Sprintf("--endpoint-url=%s", redisBackupSpec.S3EndpointUrl),
		fmt.Sprintf("--db=%d", redisBackupSpec.Db),
	}
	args = append(args, retentionArgs...)

	if redisBackupSpec.URI != "" {
		args = append(args, fmt.Sprintf("--uri=%s", redisBackupSpec.URI))
	}
	if redisBackupSpec.TTL {
		args = append(args, "--ttl")
	}
	envFrom := []corev1.EnvFromSource{}
	if redisBackupSpec.URISecretName != "" {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: redisBackupSpec.URISecretName,
				},
			},
		})
	}
	if redisBackupSpec.AWSConfigSecretName != "" {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: redisBackupSpec.AWSConfigSecretName,
				},
			},
		})
	}
	var l int32 = 3
	return &v1beta1.CronJobSpec{
		Schedule:                   redisBackupSpec.Schedule,
		SuccessfulJobsHistoryLimit: &l,
		ConcurrencyPolicy:          v1beta1.ForbidConcurrent,
		JobTemplate: v1beta1.JobTemplateSpec{
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: "Never",
						Containers: []corev1.Container{
							{
								Name:            "backup",
								Image:           redisBackupSpec.Image,
								Args:            args,
								ImagePullPolicy: "IfNotPresent",
								EnvFrom:         envFrom,
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
	spec := backupv1.RedisBackupSpec{
		Image:         rbs.Spec.Image,
		RedisType:     rbs.Spec.RedisType,
		Db:            rbs.Spec.Db,
		Bucket:        rbs.Spec.Bucket,
		S3EndpointUrl: rbs.Spec.S3EndpointUrl,
	}

	if rbs.Spec.URI != "" {
		spec.URI = rbs.Spec.URI
	}
	if rbs.Spec.AWSConfigSecretName != "" {
		spec.AWSConfigSecretName = rbs.Spec.AWSConfigSecretName
	}
	if rbs.Spec.URISecretName != "" {
		spec.URISecretName = rbs.Spec.URISecretName
	}

	return &backupv1.RedisBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: rbs.Namespace,
			Labels: map[string]string{
				"backup.yektanet.tech/redisbackupschedule": rbs.Name,
				"backup.yektanet.tech/job":                 job.Name,
			},
			Annotations: map[string]string{},
		},
		Spec: spec,
	}
}

func (r *RedisBackupScheduleReconciler) deleteExternalResources(ctx context.Context, rbs *backupv1.RedisBackupSchedule) error {
	// Delete redisBackup and cronJob
	log := log.FromContext(ctx)
	var rbl backupv1.RedisBackupList
	if err := r.List(ctx, &rbl, client.InNamespace(rbs.Namespace), client.MatchingFields{".metadata.redisbackupscheduler.controller": rbs.Name}); err != nil {
		log.Info(fmt.Sprintf("unable to list child RedisBackups %s", err))
	}

	log.Info(fmt.Sprintf("Found %d RedisBackups to delete", len(rbl.Items)))

	for _, rb := range rbl.Items {
		if err := r.Delete(ctx, &rb); err != nil {
			log.Error(err, "unable to delete RedisBackup")
			return err
		}
	}

	var cjl v1beta1.CronJobList
	if err := r.List(ctx, &cjl, client.InNamespace(rbs.Namespace), client.MatchingFields{".metadata.redisbackupscheduler.controller": rbs.Name}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err, "unable to list child CronJobs")
			return err
		}
	}

	log.Info(fmt.Sprintf("Found %d CronJobs to delete", len(cjl.Items)))

	for _, cj := range cjl.Items {
		if err := r.Delete(ctx, &cj); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err, "unable to delete CronJob")
				return err
			}
		}
	}

	var jl batchv1.JobList
	if err := r.List(ctx, &jl, client.InNamespace(rbs.Namespace), client.MatchingFields{".metadata.redisbackupscheduler.controller": rbs.Name}); err != nil {
		log.Error(err, "unable to list child Jobs")
		return err
	}

	log.Info(fmt.Sprintf("Found %d Jobs to delete", len(jl.Items)))

	for _, cj := range jl.Items {
		if err := r.Delete(ctx, &cj); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err, "unable to delete Job")
				return err
			}
		}
	}
	return nil
}

//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackupschedules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackupschedules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.yektanet.tech,resources=redisbackupschedules/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs/status,verbs=get
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;delete
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
		if !k8serrors.IsNotFound(err) {
			log.Error(err, "unable to fetch RedisBackupSchedule")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := "backup.yektanet.tech/finalizer"

	if rbs.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&rbs, finalizerName) {
			controllerutil.AddFinalizer(&rbs, finalizerName)
			if err := r.Update(ctx, &rbs); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(&rbs, finalizerName) {
			if err := r.deleteExternalResources(ctx, &rbs); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(&rbs, finalizerName)
			if err := r.Update(ctx, &rbs); err != nil {
				if !k8serrors.IsConflict(err) || !k8serrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
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
	if err := r.List(ctx, &jobs, client.InNamespace(req.Namespace), client.MatchingFields{".metadata.redisbackupscheduler.controller": req.Name}); err != nil {
		log.Error(err, "unable to list child Jobs")
		return ctrl.Result{}, err
	}

	for _, job := range jobs.Items {
		var rb backupv1.RedisBackup
		if err := r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, &rb); err != nil {
			log.Info(fmt.Sprintf("%s %s RedisBackup not found. creating RedisBackup ...", err, rbs.Name))

			rb = *getRedisBackup(&rbs, &job)

			if err := ctrl.SetControllerReference(&rbs, &rb, r.Scheme); err != nil {
				log.Error(err, "unable to set controller reference on RedisBackup")
				return ctrl.Result{}, err
			}

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
	if err := r.setupJobIndexer(mgr); err != nil {
		return err
	}

	if err := r.setupCronJobIndexer(mgr); err != nil {
		return err
	}

	if err := r.setupRedisBackupIndexer(mgr); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.RedisBackupSchedule{}).
		Owns(&backupv1.RedisBackup{}).
		Owns(&v1beta1.CronJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *RedisBackupScheduleReconciler) setupJobIndexer(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, ".metadata.redisbackupscheduler.controller", func(rawObj client.Object) []string {
		j := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(j)
		if owner == nil {
			return nil
		}

		if owner.Kind != "CronJob" {
			return nil
		}

		return []string{owner.Name}
	})
}

func (r *RedisBackupScheduleReconciler) setupCronJobIndexer(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &v1beta1.CronJob{}, ".metadata.redisbackupscheduler.controller", func(rawObj client.Object) []string {
		cj := rawObj.(*v1beta1.CronJob)
		owner := metav1.GetControllerOf(cj)
		if owner == nil {
			return nil
		}

		if owner.Kind != "RedisBackupSchedule" {
			return nil
		}

		return []string{owner.Name}
	})
}

func (r *RedisBackupScheduleReconciler) setupRedisBackupIndexer(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &backupv1.RedisBackup{}, ".metadata.redisbackupscheduler.controller", func(rawObj client.Object) []string {
		rb := rawObj.(*backupv1.RedisBackup)
		owner := metav1.GetControllerOf(rb)

		if owner == nil {
			return nil
		}

		if owner.Kind != "RedisBackupSchedule" {
			return nil
		}

		return []string{owner.Name}
	})
}
