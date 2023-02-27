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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisBackupScheduleSpec defines the desired state of RedisBackupSchedule
type RedisBackupScheduleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Image               string                           `json:"image,omitempty"`
	URI                 string                           `json:"uri,omitempty"`
	TTL                 bool                             `json:"ttl,omitempty"`
	URISecretName       string                           `json:"uriSecretName,omitempty"`
	AWSConfigSecretName string                           `json:"awsConfigSecretName,omitempty"`
	RedisType           string                           `json:"redisType,omitempty"`
	Db                  int32                            `json:"db,omitempty"`
	Schedule            string                           `json:"schedule,omitempty"`
	Bucket              string                           `json:"bucket,omitempty"`
	S3EndpointUrl       string                           `json:"s3EndpointUrl,omitempty"`
	RetentionSpec       RedisBackupScheduleRetentionSpec `json:"retentionSpec,omitempty"`
}

// RedisBackupRetentionSpec defines the desired state of backups' retention
type RedisBackupScheduleRetentionSpec struct {
	KeepLast    int32 `json:"keepLast,omitempty"`
	KeepHourly  int32 `json:"keepHourly,omitempty"`
	KeepDaily   int32 `json:"keepDaily,omitempty"`
	KeepWeekly  int32 `json:"keepWeekly,omitempty"`
	KeepMonthly int32 `json:"keepMonthly,omitempty"`
	KeepYearly  int32 `json:"keepYearly,omitempty"`
}

// RedisBackupScheduleStatus defines the observed state of RedisBackupSchedule
type RedisBackupScheduleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RedisBackupSchedule is the Schema for the redisbackupschedules API
type RedisBackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisBackupScheduleSpec   `json:"spec,omitempty"`
	Status RedisBackupScheduleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedisBackupScheduleList contains a list of RedisBackupSchedule
type RedisBackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisBackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisBackupSchedule{}, &RedisBackupScheduleList{})
}
