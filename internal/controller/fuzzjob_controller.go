/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	targetsv1alpha1 "github.com/willmurnane/clusterfuzz/api/v1alpha1"
)

// FuzzJobReconciler reconciles a FuzzJob object
type FuzzJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fuzz.will.murnane.family,resources=fuzzjobs,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=fuzz.will.murnane.family,resources=fuzzjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fuzz.will.murnane.family,resources=fuzzjobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=statefulset,resources=statefulsets/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=patch;list;watch;get
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the FuzzJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *FuzzJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	slog.Info("Reconciling", "req", req)

	saName, saErr := r.ensureServiceAccount(ctx, req.Namespace)
	if saErr != nil {
		slog.Error("Failed to ensure ServiceAccount", "name", "fuzzer", "namespace", req.Namespace, "err", saErr)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, saErr
	}
	crName, crErr := r.ensureClusterRole(ctx)
	if crErr != nil {
		slog.Error("Failed to ensure ClusterRole", "name", "fuzzer-role", "namespace", req.Namespace, "err", crErr)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, crErr
	}
	crErr = r.ensureClusterRoleBinding(ctx, crName, saName, req.Namespace)
	if crErr != nil {
		slog.Error("Failed to ensure ClusterRoleBinding", "name", "fuzzer-role-binding", "namespace", req.Namespace, "err", crErr)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, crErr
	}
	var fuzzjob targetsv1alpha1.FuzzJob
	getErr := r.Get(ctx, req.NamespacedName, &fuzzjob)
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			slog.Error("Did not locate FuzzJob", "err", getErr, "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		} else {
			slog.Error("Failed to retrieve FuzzJob", "err", getErr, "name", req.Name, "namespace", req.Namespace)
			return reconcile.Result{RequeueAfter: 5 * time.Minute}, getErr
		}
	}
	slog.Info("Found FuzzJob", "FuzzJob", fuzzjob)
	sts, stsErr := r.CreateSts(ctx, saName, &fuzzjob)
	if stsErr != nil {
		slog.Error("Failed to create StatefulSet", "name", req.Name, "namespace", req.Namespace, "err", stsErr)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, stsErr
	}

	var found appsv1.StatefulSet

	getStsErr := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, &found)
	if getStsErr != nil && apierrors.IsNotFound(getStsErr) {
		// Define a new statefulset
		slog.Info("Creating StatefulSet", "name", req.Name, "namespace", req.Namespace)
		createStsErr := r.Create(ctx, sts)
		if createStsErr != nil {
			slog.Error("Failed to create sts", "name", req.Name, "namespace", req.Namespace, "err", createStsErr, "sts", sts)
			return reconcile.Result{RequeueAfter: 5 * time.Minute}, stsErr
		}
	} else {
		slog.Info("Updating sts", "name", req.Name, "namespace", req.Namespace)
		updateStsErr := r.Update(ctx, sts)
		if updateStsErr != nil {
			slog.Error("Failed to update StatefulSet", "name", req.Name, "namespace", req.Namespace, "err", updateStsErr, "sts", sts)
			return reconcile.Result{RequeueAfter: 5 * time.Minute}, stsErr
		}
		return ctrl.Result{}, nil
	}

	// Update the Status of the FuzzJob resource
	fuzzjob.Status.Conditions = []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "FuzzJobReady",
		},
	}
	updateStatusErr := r.Status().Update(ctx, &fuzzjob)
	if updateStatusErr != nil {
		slog.Error("Failed to update FuzzJob status", "name", req.Name, "namespace", req.Namespace, "err", updateStatusErr)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, updateStatusErr
	}

	return ctrl.Result{}, nil
}

func (r *FuzzJobReconciler) ensureClusterRoleBinding(ctx context.Context, crName string, saName string, namespace string) error {
	var clusterRoleBindingName = "fuzzer-role-binding"
	getErr := r.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName}, &rbacv1.ClusterRoleBinding{})
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			slog.Info("Creating ClusterRoleBinding", "name", clusterRoleBindingName)
			clusterRoleBinding := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterRoleBindingName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      saName,
						Namespace: namespace,
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: crName,
				},
			}
			createErr := r.Create(ctx, clusterRoleBinding)
			if createErr != nil {
				slog.Error("Failed to create ClusterRoleBinding", "name", clusterRoleBindingName, "err", createErr)
				return createErr
			}
		} else {
			slog.Error("Failed to get ClusterRoleBinding", "name", clusterRoleBindingName, "err", getErr)
			return getErr
		}
	}
	return nil
}

func (r *FuzzJobReconciler) ensureClusterRole(ctx context.Context) (string, error) {
	var clusterRoleName = "clusterfuzz-fuzzer-role"
	getErr := r.Get(ctx, types.NamespacedName{Name: clusterRoleName}, &rbacv1.ClusterRole{})
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			slog.Info("Creating ClusterRole", "name", clusterRoleName)
			clusterRole := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterRoleName,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"v1"},
						Resources: []string{"pods"},
						Verbs:     []string{"patch"},
					},
				},
			}
			createErr := r.Create(ctx, clusterRole)
			if createErr != nil {
				slog.Error("Failed to create ClusterRole", "name", clusterRoleName, "err", createErr)
				return "", createErr
			}
		} else {
			slog.Error("Failed to get ClusterRole", "name", clusterRoleName, "err", getErr)
			return "", getErr
		}
	}
	return clusterRoleName, nil
}

func (r *FuzzJobReconciler) ensureServiceAccount(ctx context.Context, namespace string) (string, error) {
	var sa corev1.ServiceAccount
	getErr := r.Get(ctx, types.NamespacedName{Name: "fuzzer", Namespace: namespace}, &sa)
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			slog.Info("Creating ServiceAccount", "name", "fuzzer", "namespace", namespace)
			sa = corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fuzzer",
					Namespace: namespace,
				},
			}
			createErr := r.Create(ctx, &sa)
			if createErr != nil {
				slog.Error("Failed to create ServiceAccount", "name", "fuzzer", "namespace", namespace, "err", createErr)
				return "", createErr
			}
		} else {
			slog.Error("Failed to get ServiceAccount", "name", "fuzzer", "namespace", namespace, "err", getErr)
			return "", getErr
		}
	}
	return sa.Name, nil
}

func (r *FuzzJobReconciler) CreateContainer(index int, binary string, volumeMounts []corev1.VolumeMount) corev1.Container {
	args := []string{"exec", "cargo", "afl", "fuzz", "-i", "/in", "-o", "/data"}
	var name string
	if index == 0 {
		name = "main"
		args = append(args, "-M")
		args = append(args, "${HOSTNAME}")
	} else {
		name = fmt.Sprintf("worker%d", index)
		args = append(args, "-S")
		args = append(args, name)
	}
	// COMPCOV handling
	args = append(args, "-c")
	if index == 4 {
		args = append(args, "0")
	} else {
		args = append(args, "-")
	}
	args = append(args, binary)

	// TODO CMPLOG handling

	script := []string{
		strings.Join(args, " "),
	}
	ctr := corev1.Container{
		// TODO use a more generic image, or maybe bake afl into the controller image?
		Image:           "registry.will.murnane.family/fuzz-test:latest",
		Name:            name,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/bash", "-c"},
		Args:            script,
		Env: []corev1.EnvVar{
			{
				Name:  "AFL_NO_AFFINITY",
				Value: "1",
			},
			{
				Name:  "AFL_AUTORESUME",
				Value: "1",
			},
		},
		VolumeMounts: append(volumeMounts,
			corev1.VolumeMount{
				Name:      "corpus-disk",
				MountPath: "/data",
			},
			corev1.VolumeMount{
				Name:      "shm",
				MountPath: "/dev/shm",
			},
		),
	}
	return ctr
}

func (r *FuzzJobReconciler) CreateContainers(currentContainerImage string, count int, binary string, volumeMounts []corev1.VolumeMount) []corev1.Container {
	result := make([]corev1.Container, count+1)
	for i := range count {
		result[i] = r.CreateContainer(i, binary, volumeMounts)
	}
	result[count] = r.CreateStatsContainer(currentContainerImage, []corev1.VolumeMount{{
		Name:      "corpus-disk",
		MountPath: "/data",
	},
	})
	return result
}
func (r *FuzzJobReconciler) CreateStatsContainer(currentContainerImage string, volumeMounts []corev1.VolumeMount) corev1.Container {
	return corev1.Container{
		Name:    "update-stats",
		Image:   currentContainerImage,
		Command: []string{"/stats"},
		Env: []corev1.EnvVar{
			{
				Name:  "IMAGE",
				Value: currentContainerImage,
			},
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		VolumeMounts: volumeMounts,
	}
}
func (r *FuzzJobReconciler) CreateSts(ctx context.Context, saName string, fuzzjob *targetsv1alpha1.FuzzJob) (*appsv1.StatefulSet, error) {
	cores := fuzzjob.Spec.Cores
	var podSize int
	if cores > 32 {
		podSize = 16
	} else {
		podSize = 8
	}
	slog.Info("Creating pods", "podSize", podSize, "cores", cores)

	var retrieveTargetContainer *corev1.Container
	var targetVolume *corev1.Volume
	var targetVolumeMounts []corev1.VolumeMount
	switch fuzzjob.Spec.TargetRef.Kind {
	case "S3Target":
		var target targetsv1alpha1.S3Target
		getErr := r.Get(ctx, types.NamespacedName{Name: fuzzjob.Spec.TargetRef.Name, Namespace: fuzzjob.Namespace}, &target)
		if getErr != nil {
			slog.Error("Failed to get S3Target", "name", fuzzjob.Spec.TargetRef.Name, "namespace", fuzzjob.Namespace, "err", getErr)
			return nil, getErr
		}
		targetVolume = &corev1.Volume{
			Name: "target",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: &target.Spec.SizeLimit,
				},
			},
		}
		targetVolumeMounts = []corev1.VolumeMount{{
			Name:      "target",
			MountPath: "/target",
		}}
		env := []corev1.EnvVar{}
		if target.Spec.AccessKey != "" {
			env = append(env, corev1.EnvVar{Name: "AWS_ACCESS_KEY_ID", Value: target.Spec.AccessKey})
		}
		if target.Spec.SecretKey != "" {
			env = append(env, corev1.EnvVar{Name: "AWS_SECRET_ACCESS_KEY", Value: target.Spec.SecretKey})
		}
		if target.Spec.Region != "" {
			env = append(env, corev1.EnvVar{Name: "AWS_DEFAULT_REGION", Value: target.Spec.Region})

		}
		var sb strings.Builder
		for arch, bin := range target.Spec.Binaries {
			var prefix string
			if target.Spec.Endpoint == "" {
				prefix = "aws"
			} else {
				prefix = fmt.Sprintf("aws --endpoint %s", target.Spec.Endpoint)
			}
			var source string
			if strings.HasPrefix(bin, "/") {
				source = fmt.Sprintf("s3://%s%s", target.Spec.Bucket, bin)
			} else {
				source = fmt.Sprintf("s3://%s/%s", target.Spec.Bucket, bin)
			}
			cmd := fmt.Sprintf("%s --debug s3 cp %s /target/%s\n", prefix, source, arch)
			sb.WriteString(cmd)
			sb.WriteString(fmt.Sprintf("chmod a+x /target/%s", arch))
		}
		retrieveTargetContainer = &corev1.Container{
			Name:         "get-binary",
			Image:        "public.ecr.aws/aws-cli/aws-cli:latest",
			Env:          env,
			Command:      []string{"/bin/bash", "-xc"},
			Args:         []string{sb.String()},
			VolumeMounts: targetVolumeMounts,
		}
	case "ImageTarget":
		var target targetsv1alpha1.ImageTarget
		getErr := r.Get(ctx, types.NamespacedName{Name: fuzzjob.Spec.TargetRef.Name, Namespace: fuzzjob.Namespace}, &target)
		if getErr != nil {
			slog.Error("Failed to get ImageTarget", "name", fuzzjob.Spec.TargetRef.Name, "namespace", fuzzjob.Namespace, "err", getErr)
			return nil, getErr
		}
		targetVolume = &corev1.Volume{
			Name: "target",
			VolumeSource: corev1.VolumeSource{
				Image: &corev1.ImageVolumeSource{
					Reference:  *target.Spec.Image,
					PullPolicy: corev1.PullAlways,
				},
			},
		}
		targetVolumeMounts = []corev1.VolumeMount{{
			Name:      "target",
			MountPath: "/target",
		}}
		retrieveTargetContainer = nil
	}

	currentContainerImage := os.Getenv("CONTROLLER_IMAGE")
	// FIXME hard-coded arch here. And what about asan?
	containers := r.CreateContainers(currentContainerImage, podSize, "/target/amd64", targetVolumeMounts)
	initContainers := []corev1.Container{}

	if retrieveTargetContainer != nil {
		initContainers = append(initContainers, *retrieveTargetContainer)
	}
	volumes := []corev1.Volume{
		{
			Name: "shm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(1, "Gi"),
					Medium:    corev1.StorageMediumMemory,
				},
			},
		},
	}
	if targetVolume != nil {
		volumes = append(volumes, *targetVolume)
	}

	replicas := int32(cores / podSize)
	// have to run as root for afl to work properly
	uid := int64(0)
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fuzzjob.Name,
			Namespace: fuzzjob.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: fuzzjob.APIVersion,
					Kind:       fuzzjob.Kind,
					Name:       fuzzjob.Name,
					UID:        fuzzjob.UID,
				},
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "afl"},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					InitContainers:     initContainers,
					Containers:         containers,
					Volumes:            volumes,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &uid,
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "afl"},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "corpus-disk",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("50Gi"),
							},
						},
					},
				},
			},
		},
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FuzzJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&targetsv1alpha1.FuzzJob{}).
		Named("fuzzjob").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
