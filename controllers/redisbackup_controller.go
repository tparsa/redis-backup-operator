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
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

func getRedisBackupJobSpec(rb *backupv1.RedisBackup) *batchv1.JobSpec {
	redisBackupSpec := rb.Spec
	return &batchv1.JobSpec{
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
	}
}

func getRedisBackupJobWithName(rb *backupv1.RedisBackup, name string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rb.Namespace,
			Labels: map[string]string{
				"backup.yektanet.tech/redisbackup": rb.Name,
			},
		},
		Spec: *getRedisBackupJobSpec(rb),
	}
}

func getRedisBackupJob(rb *backupv1.RedisBackup) *batchv1.Job {
	return getRedisBackupJobWithName(rb, fmt.Sprintf("%s-%d", rb.Name, time.Now().Unix()))
}

func getRedisBackupJobPatch(rb *backupv1.RedisBackup) *batchv1.Job {
	return &batchv1.Job{
		Spec: *getRedisBackupJobSpec(rb),
	}
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
	var rb backupv1.RedisBackup
	if err := r.Get(ctx, req.NamespacedName, &rb); err != nil {
		log.Error(err, "unable to fetch RedisBackup")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if _, ok := rb.Labels["backup.yektanet.tech/job"]; !ok {
		job := *getRedisBackupJob(&rb)
		if err := r.Create(ctx, &job); err != nil {
			log.Error(err, "unable to create Job for RedisBackup", "job", job)
			return ctrl.Result{}, err
		}
	} else {
		jobName := rb.Labels["backup.yektanet.tech/job"]
		var job batchv1.Job
		if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
			log.Info(fmt.Sprintf("%s %s Job not found. creating Job ...", err, rb.Name))
			job = *getRedisBackupJobWithName(&rb, jobName)
			if err := r.Create(ctx, &job); err != nil {
				log.Error(err, "unable to create Job for RedisBackup", "job", job)
				return ctrl.Result{}, err
			}
			patchObj := metav1.ObjectMeta{Labels: map[string]string{"backup.yektanet.tech/job": job.Name}}
			patch, err := json.Marshal(patchObj)
			if err != nil {
				log.Error(err, "Unable to marshal patch", "patchObj", patchObj)
				return ctrl.Result{}, err
			}
			if err := r.Patch(ctx, &rb, client.RawPatch(types.MergePatchType, patch)); err != nil {
				log.Error(err, "Unable to patch RedisBackup", "redisbackup", rb)
				return ctrl.Result{}, err
			}
		} else {
			patchObj := *getRedisBackupJobPatch(&rb)
			patch, err := json.Marshal(patchObj)
			if err != nil {
				log.Error(err, "Unable to marshal patch", "patchObj", patchObj)
				return ctrl.Result{}, err
			}

			if err := r.Patch(ctx, &job, client.RawPatch(types.MergePatchType, patch)); err != nil {
				log.Error(err, "Couldn't patch Job for RedisBackup", "job", job)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.RedisBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
