package traffic

import (
	"context"
	"fmt"
	"time"

	autoscalerv1beta1 "github.com/aledbf/horus/pkg/apis/autoscaler/v1beta1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerutil "sigs.k8s.io/controller-runtime"
)

func (r *ReconcileTraffic) ensureProxyRunning(instance *autoscalerv1beta1.Traffic) error {
	if err := r.reconcileProxyDeployment(instance); err != nil {
		return errors.Wrap(err, "reconcile proxy deployment")
	}

	if err := r.waitForProxyDeploymentReady(instance); err != nil {
		return errors.Wrap(err, "waiting for proxy deployment to be ready")
	}

	if err := r.reconcileProxyService(instance); err != nil {
		return errors.Wrap(err, "reconcile proxy service")
	}

	return nil
}

func (r *ReconcileTraffic) reconcileProxyService(instance *autoscalerv1beta1.Traffic) error {
	serviceName := proxyServiceName(instance)

	foundService := &corev1.Service{}
	err := r.Get(context.TODO(), serviceName, foundService)
	if err != nil {
		return errors.Wrap(err, "get service")
	}

	return nil
}

func (r *ReconcileTraffic) reconcileProxyDeployment(instance *autoscalerv1beta1.Traffic) error {
	deploymentName := proxyDeploymentName(instance)

	var service *corev1.Service

	labels := extractServiceLabels(service)
	labels["kind"] = "horus-proxy"

	replicas := int32(1)

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v-horus-proxy", deploymentName.Name),
			Namespace: deploymentName.Namespace,
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
							Ports:           extractServicePorts(service),
							Env: []corev1.EnvVar{
								{
									Name:  "PROXY_NAMESPACE",
									Value: instance.Namespace,
								},
								{
									Name:  "PROXY_SERVICE",
									Value: instance.Spec.Service,
								},
							},
						},
					},
				},
			},
		},
	}

	foundDeployment := &appsv1.Deployment{}
	err := r.Get(context.TODO(), deploymentName, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		err := r.Create(context.TODO(), deployment)
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

func (r *ReconcileTraffic) waitForProxyDeploymentReady(instance *autoscalerv1beta1.Traffic) error {
	abortAt := time.Now().Add(time.Minute * 2)
	for {
		if time.Now().After(abortAt) {
			return errors.Wrap(fmt.Errorf("timeout waiting for proxy to be ready"), "waiting for proxy")
		}

		isReady, err := r.isProxyDeploymentReady(instance)
		if err != nil {
			return errors.Wrap(err, "is proxy ready")
		}

		if isReady {
			return nil
		}
	}
}

func (r *ReconcileTraffic) isProxyDeploymentReady(instance *autoscalerv1beta1.Traffic) (bool, error) {
	deploymentName := proxyDeploymentName(instance)

	foundDeployment := &appsv1.Deployment{}
	err := r.Get(context.TODO(), deploymentName, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "get deployment to check status")
	}

	return foundDeployment.Status.AvailableReplicas > 0, nil
}

func proxyServiceName(instance *autoscalerv1beta1.Traffic) types.NamespacedName {
	return types.NamespacedName{
		Name:      instance.Spec.Service,
		Namespace: instance.Namespace,
	}
}

func proxyDeploymentName(instance *autoscalerv1beta1.Traffic) types.NamespacedName {
	return types.NamespacedName{
		Name:      fmt.Sprintf("%v-proxy", instance.Spec.Deployment),
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
