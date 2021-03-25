package injector

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func getInitContainerSpec(containerName string, containerImage string, outboundIPRangeExclusionList []string, enablePrivilegedInitContainer bool) corev1.Container {
	// local fix: set resource limitation
	cpuReq, _ := resource.ParseQuantity("10m")
	cpuLmt, _ := resource.ParseQuantity("30m")
	memReq, _ := resource.ParseQuantity("20Mi")
	memLmt, _ := resource.ParseQuantity("50Mi")
	iptablesInitCommandsList := generateIptablesCommands(outboundIPRangeExclusionList)
	iptablesInitCommand := strings.Join(iptablesInitCommandsList, " && ")

	return corev1.Container{
		Name:  containerName,
		Image: containerImage,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &enablePrivilegedInitContainer,
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
				},
			},
		},
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			iptablesInitCommand,
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
