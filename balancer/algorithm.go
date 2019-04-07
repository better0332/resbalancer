package balancer

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/priorities"
)

var priorityConfig = []priorities.PriorityConfig{
	{
		Name:   priorities.RequestedToCapacityRatioPriority,
		Map:    priorities.RequestedToCapacityRatioResourceAllocationPriorityDefault().PriorityMap,
		Weight: 1,
	},
	{
		Name:   priorities.LeastRequestedPriority,
		Map:    priorities.LeastRequestedPriorityMap,
		Weight: 1,
	},
	{
		Name:   priorities.BalancedResourceAllocation,
		Map:    priorities.BalancedResourceAllocationMap,
		Weight: 1,
	},
}

var fakePod = &v1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "fakePod",
		Namespace: metav1.NamespaceDefault,
	},
	Spec: v1.PodSpec{
		Containers: []v1.Container{
			{
				Image: "image/fake",
			},
		},
	},
}
