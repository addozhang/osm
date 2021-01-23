package injector

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"

	"github.com/openservicemesh/osm/pkg/constants"
)

func getInitContainerSpec(containerName, containerImage string) corev1.Container {
	cpuReq, _ := resource.ParseQuantity("50m")
	cpuLmt, _ := resource.ParseQuantity("200m")
	memReq, _ := resource.ParseQuantity("100Mi")
	memLmt, _ := resource.ParseQuantity("500Mi")
	//privileged := true
	return corev1.Container{
		Name:  containerName,
		Image: containerImage,
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
				},
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "OSM_PROXY_UID",
				Value: fmt.Sprintf("%d", constants.EnvoyUID),
			},
			{
				Name:  "OSM_ENVOY_INBOUND_PORT",
				Value: fmt.Sprintf("%d", constants.EnvoyInboundListenerPort),
			},
			{
				Name:  "OSM_ENVOY_OUTBOUND_PORT",
				Value: fmt.Sprintf("%d", constants.EnvoyOutboundListenerPort),
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    cpuLmt,
				corev1.ResourceMemory: memLmt,
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    cpuReq,
				corev1.ResourceMemory: memReq,
			},
		},
	}
}
