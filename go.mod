module github.com/aledbf/horus

go 1.12

require (
	github.com/go-logr/logr v0.1.0
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	k8s.io/api v0.0.0-20190703205437-39734b2a72fe
	k8s.io/apimachinery v0.0.0-20190703205208-4cfb76a8bf76

	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.0.0-20190612125529-c522cb6c26aa
	k8s.io/klog v0.3.3 // indirect
	k8s.io/kube-openapi v0.0.0-20190418160015-6b3d3b2d5666 // indirect
	sigs.k8s.io/controller-runtime v0.2.0-beta.4
)
