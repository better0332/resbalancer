package balancer

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return pod.Spec.NodeName != ""
}

// we only focus on ReplicaSet, because its stateless
func canDeletePod(pod *v1.Pod) bool {
	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef.Kind == "ReplicaSet" && pod.DeletionTimestamp == nil && pod.Spec.Affinity == nil &&
		pod.Spec.NodeName != "" && len(pod.Spec.NodeSelector) == 0 {
		return true
	}
	return false
}

func workNode(node *v1.Node) bool {
	// ignore unschedulable and taints node
	if node.DeletionTimestamp != nil || node.Spec.Unschedulable || len(node.Spec.Taints) > 0 {
		return false
	}

	for _, c := range node.Status.Conditions {
		switch c.Type {
		case v1.NodeReady:
			return c.Status == v1.ConditionTrue
		default:
			// We ignore other conditions.
		}
	}
	return false
}

func resPressureNode(node *v1.Node) bool {
	for _, c := range node.Status.Conditions {
		switch c.Type {
		case v1.NodeMemoryPressure:
			return c.Status == v1.ConditionTrue
		case v1.NodeDiskPressure:
			return c.Status == v1.ConditionTrue
		case v1.NodePIDPressure:
			return c.Status == v1.ConditionTrue
		default:
			// We ignore other conditions.
		}
	}
	return false
}
