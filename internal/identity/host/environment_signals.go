package identity

import (
	"strings"
)

func joinVersionParts(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, " ")
}

func normalizeHardwareEvidence(manufacturer, product string) (virtualizationVendor, physicalHardwareHint string) {
	combined := strings.ToLower(strings.TrimSpace(manufacturer + " " + product))
	for _, marker := range []struct{ text, name string }{
		{"vmware", "vmware"},
		{"virtualbox", "virtualbox"},
		{"kvm", "kvm"},
		{"qemu", "qemu"},
		{"virtual machine", "hyper-v"},
		{"amazon ec2", "ec2"},
		{"google compute engine", "gce"},
		{"openstack", "openstack"},
	} {
		if strings.Contains(combined, marker.text) {
			return marker.name, ""
		}
	}

	vendor := strings.ToLower(strings.TrimSpace(manufacturer))
	for _, marker := range []struct{ text, name string }{
		{"dell", "dell"},
		{"hewlett-packard", "hp"},
		{"hewlett packard", "hp"},
		{"hpe", "hpe"},
		{"lenovo", "lenovo"},
		{"supermicro", "supermicro"},
		{"apple", "apple"},
		{"asus", "asus"},
		{"acer", "acer"},
	} {
		if strings.Contains(vendor, marker.text) {
			return "", marker.name
		}
	}
	return "", ""
}

func detectKubernetesSignals(env map[string]string) platformEnvironmentSignals {
	serviceHost := strings.TrimSpace(env["KUBERNETES_SERVICE_HOST"])
	podUID := strings.TrimSpace(env["POD_UID"])
	servicePort := strings.TrimSpace(env["KUBERNETES_SERVICE_PORT"])
	detected := serviceHost != "" || podUID != "" || servicePort != ""
	return platformEnvironmentSignals{
		KubernetesDetected:       detected,
		KubernetesPodIdentity:    detected,
		KubernetesDetectionKnown: detected || strings.EqualFold(strings.TrimSpace(env["AR_IMMS_ORCHESTRATOR"]), "NONE"),
	}
}
