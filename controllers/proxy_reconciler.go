package controllers

import (
	"context"
	"fmt"
	"reflect"
	"time"

	autoscalerv1beta1 "github.com/aledbf/horus/api/v1beta1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	controllerutil "sigs.k8s.io/controller-runtime"
)

const (
	handledByLabelName  = "handled-by"
	handledByLabelValue = "horus-proxy"
)

func (r *TrafficReconciler) ensureProxyRunning(instance *autoscalerv1beta1.Traffic, service *corev1.Service) error {
	fixNamespace(instance)

	if err := r.reconcileServiceAccount(instance); err != nil {
		return errors.Wrap(err, "reconciling service account")
	}

	if err := r.reconcileRoles(instance); err != nil {
		return errors.Wrap(err, "reconciling roles")
	}

	if err := r.reconcileRoleBinding(instance); err != nil {
		return errors.Wrap(err, "reconciling role binding")
	}

	labels := service.Labels
	labels[handledByLabelName] = handledByLabelValue

	ports := extractServicePorts(service)

	if err := r.reconcileProxyDeployment(instance, labels, ports); err != nil {
		return errors.Wrap(err, "reconciling proxy deployment")
	}

	if err := r.waitForProxyDeploymentReady(instance); err != nil {
		return errors.Wrap(err, "waiting for proxy deployment to be ready")
	}

	if err := r.reconcileProxyService(instance, labels); err != nil {
		return errors.Wrap(err, "updating service")
	}

	return nil
}

func (r *TrafficReconciler) reconcileProxyService(instance *autoscalerv1beta1.Traffic,
	labels map[string]string) error {
	foundService := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{
		Name:      instance.Spec.Service,
		Namespace: instance.Namespace,
	}, foundService)
	if err != nil {
		return errors.Wrap(err, "get service")
	}

	if reflect.DeepEqual(foundService.Spec.Selector, labels) {
		return nil
	}

	r.Recorder.Event(instance, "Normal", "Updated",
		fmt.Sprintf("Updated service %s/%s labels to use horus-proxy",
			instance.Namespace, instance.Spec.Service))

	foundService.Labels[handledByLabelName] = handledByLabelValue
	foundService.Spec.Selector = labels
	err = r.Update(context.TODO(), foundService)
	if err != nil {
		return err
	}

	return nil
}

func (r *TrafficReconciler) reconcileProxyDeployment(instance *autoscalerv1beta1.Traffic,
	labels map[string]string, ports []corev1.ContainerPort) error {
	replicas := int32(1)

	idleAfter := "90s"
	if instance.Spec.IdleAfter != nil {
		idleAfter = instance.Spec.IdleAfter.Duration.String()
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toProxyName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: toProxyName(instance),
					Containers: []corev1.Container{
						{
							Name:            "proxy",
							Image:           "aledbf/horus-proxy:dev",
							ImagePullPolicy: corev1.PullAlways,
							Ports:           ports,
							Env: []corev1.EnvVar{
								{
									Name:  "PROXY_NAMESPACE",
									Value: instance.Namespace,
								},
								{
									Name:  "PROXY_SERVICE",
									Value: instance.Spec.Service,
								},
								{
									Name:  "PROXY_DEPLOYMENT",
									Value: instance.Spec.Deployment,
								},
								{
									Name:  "IDLE_AFTER",
									Value: idleAfter,
								},
							},
						},
					},
				},
			},
		},
	}

	foundDeployment := &appsv1.Deployment{}
	err := r.Get(context.TODO(), typeNamespace(instance), foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), deployment)
		if err != nil {
			return err
		}

		r.Recorder.Event(instance, "Normal", "Created",
			fmt.Sprintf("New horus-proxy deployment for traffic deployment %s and service %s in namespace %s",
				instance.Spec.Deployment, instance.Spec.Service, instance.Namespace))

	}
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(deployment.Spec, foundDeployment.Spec) {
		err = r.Update(context.TODO(), deployment)
		if err != nil {
			return err
		}
	}

	if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
		return err
	}

	return nil
}

func (r *TrafficReconciler) reconcileServiceAccount(instance *autoscalerv1beta1.Traffic) error {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toProxyName(instance),
			Namespace: instance.Namespace,
		},
	}

	foundServiceAccount := &corev1.ServiceAccount{}
	err := r.Get(context.TODO(), typeNamespace(instance), foundServiceAccount)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), serviceAccount)
		if err != nil {
			return err
		}
	} else if err != nil && apierrors.IsAlreadyExists(err) {
		if !reflect.DeepEqual(serviceAccount, foundServiceAccount) {
			err = r.Update(context.TODO(), serviceAccount)
			if err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, serviceAccount, r.Scheme); err != nil {
		return err
	}

	return nil
}

func (r *TrafficReconciler) reconcileRoles(instance *autoscalerv1beta1.Traffic) error {
	roles := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "Role"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      toProxyName(instance),
			Namespace: instance.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"namespaces"},
				Verbs:         []string{"get"},
				ResourceNames: []string{instance.GetNamespace()},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				Verbs:         []string{"get", "watch", "list"},
				ResourceNames: []string{},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"pods"},
				Verbs:         []string{"get", "watch", "list"},
				ResourceNames: []string{},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				Verbs:         []string{"update"},
				ResourceNames: []string{instance.Spec.Service},
			},
			{
				APIGroups:     []string{"apps"},
				Resources:     []string{"deployments"},
				Verbs:         []string{"get", "watch", "list", "update"},
				ResourceNames: []string{},
			},
		},
	}

	foundRoles := &rbacv1.Role{}
	err := r.Get(context.TODO(), typeNamespace(instance), foundRoles)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), roles)
		if err != nil {
			return err
		}
	} else if err != nil && apierrors.IsAlreadyExists(err) {
		if !reflect.DeepEqual(roles, foundRoles) {
			err = r.Update(context.TODO(), roles)
			if err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, roles, r.Scheme); err != nil {
		return err
	}

	return nil
}

func (r *TrafficReconciler) reconcileRoleBinding(instance *autoscalerv1beta1.Traffic) error {
	name := toProxyName(instance)
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			Name: name, APIGroup: rbacv1.GroupName, Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Name: name, Namespace: instance.Namespace, APIGroup: corev1.GroupName, Kind: "ServiceAccount",
			},
		},
	}

	foundRoleBinding := &rbacv1.RoleBinding{}
	err := r.Get(context.TODO(), typeNamespace(instance), foundRoleBinding)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), roleBinding)
		if err != nil {
			return err
		}
	} else if err != nil && apierrors.IsAlreadyExists(err) {
		if !reflect.DeepEqual(roleBinding, foundRoleBinding) {
			err = r.Update(context.TODO(), roleBinding)
			if err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, roleBinding, r.Scheme); err != nil {
		return err
	}

	return nil
}

func (r *TrafficReconciler) waitForProxyDeploymentReady(instance *autoscalerv1beta1.Traffic) error {
	return wait.Poll(2*time.Second, 2*time.Minute, func() (bool, error) {
		isReady, err := r.isProxyDeploymentReady(instance)
		if err != nil {
			return false, nil
		}

		if isReady {
			return true, nil
		}

		return false, nil
	})
}

func (r *TrafficReconciler) isProxyDeploymentReady(instance *autoscalerv1beta1.Traffic) (bool, error) {
	deploymentName := typeNamespace(instance)

	foundDeployment := &appsv1.Deployment{}
	err := r.Get(context.TODO(), deploymentName, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "get deployment to check status")
	}

	return foundDeployment.Status.AvailableReplicas > 0, nil
}

func typeNamespace(instance *autoscalerv1beta1.Traffic) types.NamespacedName {
	return types.NamespacedName{
		Name:      toProxyName(instance),
		Namespace: instance.Namespace,
	}
}

func extractServicePorts(svc *corev1.Service) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{}
	for _, port := range svc.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.TargetPort.IntVal,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	return ports
}

func toProxyName(instance *autoscalerv1beta1.Traffic) string {
	return fmt.Sprintf("%v-%v-horus-proxy", instance.Spec.Deployment, instance.Spec.Service)
}

func fixNamespace(instance *autoscalerv1beta1.Traffic) {
	if instance.Namespace == "" {
		instance.Namespace = metav1.NamespaceDefault
	}
}
