/*

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

	autoscalerv1beta1 "github.com/aledbf/horus/api/v1beta1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName("controller")

// TrafficReconciler reconciles a Traffic object
type TrafficReconciler struct {
	client.Client

	Recorder record.EventRecorder

	Log logr.Logger

	Scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Traffic object and makes changes based on the state read
// and what is in the Traffic.Spec
//
// Automatically generate RBAC rules
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;get;list;watch;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=create;get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=create;get;list;watch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=create;get;list;watch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=create;get;list;watch;delete
// +kubebuilder:rbac:groups=autoscaler.rocket-science.io,resources=traffics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.rocket-science.io,resources=traffics/status,verbs=get;update;patch

func (r *TrafficReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("traffic", request.NamespacedName)

	instance := &autoscalerv1beta1.Traffic{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	finalizerName := "traffic.finalizers.rocket-science.io"

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !containsString(instance.ObjectMeta.Finalizers, finalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(instance.ObjectMeta.Finalizers, finalizerName) {
			// our finalizer is present, so lets handle our external dependency
			if err := r.deleteExternalDependency(instance); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return reconcile.Result{}, err
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = removeString(instance.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}

			r.Recorder.Event(instance, "Normal", "Deleted",
				fmt.Sprintf("Removed horus-proxy labels from service %s/%s",
					instance.Namespace, instance.Spec.Service))
		}

		// Our finalizer has finished, so the reconciler can do nothing.
		return reconcile.Result{}, nil
	}

	service := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{
		Name:      instance.Spec.Service,
		Namespace: instance.Namespace,
	}, service)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.ensureProxyRunning(instance, service)

	return reconcile.Result{}, err
}

func (r *TrafficReconciler) deleteExternalDependency(instance *autoscalerv1beta1.Traffic) error {
	log.Info("deleting the external dependencies")

	foundService := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{
		Name:      instance.Spec.Service,
		Namespace: instance.Namespace,
	}, foundService)
	if err != nil {
		return err
	}

	if kind, ok := foundService.Spec.Selector[handledByLabelName]; ok && kind == handledByLabelValue {
		delete(foundService.Spec.Selector, handledByLabelName)
		delete(foundService.Labels, handledByLabelName)

		err = r.Update(context.TODO(), foundService)
		if err != nil {
			return err
		}

		r.Recorder.Event(instance, "Normal", "Updated",
			fmt.Sprintf("Updated service %v/%v selector to remove horus-proxy",
				instance.Namespace, instance.Spec.Service))
	}

	key := typeNamespace(instance)

	err = r.deleteResource(key, &appsv1.Deployment{})
	if err != nil {
		return err
	}

	r.Recorder.Event(instance, "Normal", "Deleted",
		fmt.Sprintf("Removed horus-proxy deployment for deployment %s and service %s in namespace %s",
			instance.Spec.Deployment, instance.Spec.Service, instance.Namespace))

	err = r.deleteResource(key, &rbacv1.RoleBinding{})
	if err != nil {
		return err
	}

	r.Recorder.Event(instance, "Normal", "Deleted",
		fmt.Sprintf("Removed horus-proxy role binding for deployment %s and service %s in namespace %s",
			instance.Spec.Deployment, instance.Spec.Service, instance.Namespace))

	err = r.deleteResource(key, &rbacv1.Role{})
	if err != nil {
		return err
	}

	r.Recorder.Event(instance, "Normal", "Deleted",
		fmt.Sprintf("Removed horus-proxy role for deployment %s and service %s in namespace %s",
			instance.Spec.Deployment, instance.Spec.Service, instance.Namespace))

	err = r.deleteResource(key, &corev1.ServiceAccount{})
	if err != nil {
		return err
	}

	r.Recorder.Event(instance, "Normal", "Deleted",
		fmt.Sprintf("Removed horus-proxy service account for deployment %s and service %s in namespace %s",
			instance.Spec.Deployment, instance.Spec.Service, instance.Namespace))

	return nil
}

func (r *TrafficReconciler) deleteResource(key types.NamespacedName, obj runtime.Object) error {
	err := r.Get(context.TODO(), key, obj)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	err = r.Delete(context.TODO(), obj)
	if err != nil {
		return err
	}

	return nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func (r *TrafficReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalerv1beta1.Traffic{}).
		Complete(r)
}
