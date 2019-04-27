package traffic

import (
	"context"
	"fmt"
	"time"

	autoscalerv1beta1 "github.com/aledbf/horus/pkg/apis/autoscaler/v1beta1"
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

func (r *ReconcileTraffic) ensureProxyRunning(instance *autoscalerv1beta1.Traffic, service *corev1.Service) error {
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

	labels := extractServiceLabels(service)
	labels["kind"] = "horus-proxy"

	ports := extractServicePorts(service)

	if err := r.reconcileProxyDeployment(instance, labels, ports); err != nil {
		return errors.Wrap(err, "reconciling proxy deployment")
	}

	if err := r.waitForProxyDeploymentReady(instance); err != nil {
		return errors.Wrap(err, "waiting for proxy deployment to be ready")
	}

	if err := r.reconcileProxyService(instance); err != nil {
		return errors.Wrap(err, "updating service")
	}

	return nil
}

func (r *ReconcileTraffic) reconcileProxyService(instance *autoscalerv1beta1.Traffic) error {
	serviceName := typeNamespace(instance)
	foundService := &corev1.Service{}
	err := r.Get(context.TODO(), serviceName, foundService)
	if err != nil {
		return errors.Wrap(err, "get service")
	}

	return nil
}

func (r *ReconcileTraffic) reconcileProxyDeployment(instance *autoscalerv1beta1.Traffic, labels map[string]string, ports []corev1.ContainerPort) error {
	deploymentName := typeNamespace(instance)

	replicas := int32(1)

	foundDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      toProxyName(deploymentName.Name),
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
					Name:   deploymentName.Name,
					Labels: labels,
				},
				Spec: corev1.PodSpec{
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
							},
						},
					},
				},
			},
		},
	}

	err := r.Get(context.TODO(), deploymentName, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), foundDeployment)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, foundDeployment, r.scheme); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileTraffic) reconcileServiceAccount(instance *autoscalerv1beta1.Traffic) error {
	foundServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toProxyName(instance.Spec.Service),
			Namespace: instance.Namespace,
		},
	}
	err := r.Get(context.TODO(), getKey(instance), foundServiceAccount)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), foundServiceAccount)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, foundServiceAccount, r.scheme); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileTraffic) reconcileRoles(instance *autoscalerv1beta1.Traffic) error {
	foundRoles := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "Role"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      toProxyName(instance.Spec.Service),
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
				Resources:     []string{"services", "endpoints"},
				Verbs:         []string{"get", "watch", "list"},
				ResourceNames: []string{instance.Spec.Service},
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
				Verbs:         []string{"get", "watch", "list"},
				ResourceNames: []string{instance.Spec.Deployment},
			},
		},
	}

	err := r.Get(context.TODO(), getKey(instance), foundRoles)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), foundRoles)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, foundRoles, r.scheme); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileTraffic) reconcileRoleBinding(instance *autoscalerv1beta1.Traffic) error {
	name := toProxyName(instance.Spec.Service)
	foundRoleBinding := &rbacv1.RoleBinding{
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

	err := r.Get(context.TODO(), getKey(instance), foundRoleBinding)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), foundRoleBinding)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(instance, foundRoleBinding, r.scheme); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileTraffic) waitForProxyDeploymentReady(instance *autoscalerv1beta1.Traffic) error {
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

func (r *ReconcileTraffic) isProxyDeploymentReady(instance *autoscalerv1beta1.Traffic) (bool, error) {
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
		Name:      instance.Spec.Service,
		Namespace: instance.Namespace,
	}
}

func extractServiceLabels(svc *corev1.Service) map[string]string {
	labels := make(map[string]string, len(svc.Labels))
	for k, v := range svc.Labels {
		labels[fmt.Sprintf("%v-horus-proxy", k)] = v
	}

	return labels
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

func toProxyName(name string) string {
	return fmt.Sprintf("%v-horus-proxy", name)
}

func fixNamespace(instance *autoscalerv1beta1.Traffic) {
	if instance.Namespace == "" {
		instance.Namespace = metav1.NamespaceDefault
	}
}

func getKey(instance *autoscalerv1beta1.Traffic) types.NamespacedName {
	return types.NamespacedName{
		Name:      toProxyName(instance.GetName()),
		Namespace: instance.Namespace,
	}
}
