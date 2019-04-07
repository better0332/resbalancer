package balancer

import (
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/klog"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"
	schedulercore "k8s.io/kubernetes/pkg/scheduler/core"
	"k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

type NodeInfoExt struct {
	*nodeinfo.NodeInfo
	score int
}

func (n *NodeInfoExt) AddPod(pod *v1.Pod) {
	if n == nil {
		return
	}
	n.NodeInfo.AddPod(pod)
	n.score, _ = CalcPrioritizeNodeScore(n.NodeInfo)
}

func (n *NodeInfoExt) RemovePod(pod *v1.Pod) {
	if n == nil {
		return
	}
	n.NodeInfo.RemovePod(pod)
	n.score, _ = CalcPrioritizeNodeScore(n.NodeInfo)
}

func (n *NodeInfoExt) GetScore() float64 {
	if n == nil {
		return 0
	}
	return float64(n.score)
}

type nodeCache struct {
	sync.Mutex

	cache map[string]*nodeinfo.NodeInfo
}

func newNodeCache() *nodeCache {
	return &nodeCache{cache: make(map[string]*nodeinfo.NodeInfo)}
}

func (nc *nodeCache) NewNode(name string) {
	if name == "" {
		return
	}

	klog.V(6).Infof("NewNode: %v", name)

	nc.Lock()
	defer nc.Unlock()

	if _, ok := nc.cache[name]; !ok {
		nc.cache[name] = nodeinfo.NewNodeInfo()
	}
}

func (nc *nodeCache) AddNode(obj interface{}) {
	n, ok := obj.(*v1.Node)
	if !ok {
		klog.Errorf("cannot convert to *v1.Node: %v", obj)
		return
	}

	klog.V(6).Infof("AddNode: %v", n.Name)

	nc.Lock()
	defer nc.Unlock()

	if _, ok := nc.cache[n.Name]; !ok {
		nc.cache[n.Name] = nodeinfo.NewNodeInfo()
	}
	nc.cache[n.Name].SetNode(n)
}

func (nc *nodeCache) UpdateNode(_ interface{}, newObj interface{}) {
	nc.AddNode(newObj)
}

func (nc *nodeCache) DeleteNode(obj interface{}) {
	n, ok := obj.(*v1.Node)
	if !ok {
		klog.Errorf("cannot convert to *v1.Node: %v", obj)
		return
	}

	klog.V(6).Infof("DeleteNode: %v", n.Name)

	nc.Lock()
	defer nc.Unlock()

	delete(nc.cache, n.Name)
}

func (nc *nodeCache) AddPod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		klog.Errorf("cannot convert to *v1.Pod: %v", obj)
		return
	}

	if pod.Spec.NodeName == "" {
		return
	}

	klog.V(6).Infof("AddPod: %v", pod.Name)

	nc.Lock()
	defer nc.Unlock()

	if info, ok := nc.cache[pod.Spec.NodeName]; ok {
		info.AddPod(pod)
	}
}

func (nc *nodeCache) UpdatePod(oldObj interface{}, newObj interface{}) {
	oldPod, ok := oldObj.(*v1.Pod)
	if !ok {
		klog.Errorf("cannot convert oldObj to *v1.Pod: %v", oldObj)
		return
	}
	newPod, ok := newObj.(*v1.Pod)
	if !ok {
		klog.Errorf("cannot convert newObj to *v1.Pod: %v", newObj)
		return
	}

	if (oldPod.Spec.NodeName != newPod.Spec.NodeName) || oldPod.Spec.NodeName == "" {
		return
	}

	klog.V(6).Infof("UpdatePod: oldPod(%v), newPod(%v)", oldPod.Name, newPod.Name)

	nc.Lock()
	defer nc.Unlock()

	if info, ok := nc.cache[oldPod.Spec.NodeName]; ok {
		info.RemovePod(oldPod)
		info.AddPod(newPod)
	}
}

func (nc *nodeCache) DeletePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		klog.Errorf("cannot convert to *v1.Pod: %v", obj)
		return
	}

	nc.Lock()
	defer nc.Unlock()

	klog.V(6).Infof("DeletePod: %v", pod.Name)

	if info, ok := nc.cache[pod.Spec.NodeName]; ok {
		info.RemovePod(pod)
	}
}

func (nc *nodeCache) GetResPressureNode() []string {
	nc.Lock()
	defer nc.Unlock()

	nodes := make([]string, 0)
	for name, info := range nc.cache {
		if info.MemoryPressureCondition() == v1.ConditionTrue ||
			info.DiskPressureCondition() == v1.ConditionTrue ||
			info.PIDPressureCondition() == v1.ConditionTrue {
			nodes = append(nodes, name)
		}
	}
	return nodes
}

func (nc *nodeCache) FilterNode(filterFunc func(node *v1.Node) bool) []string {
	nc.Lock()
	defer nc.Unlock()

	nodes := make([]string, 0)
	for name, info := range nc.cache {
		if filterFunc(info.Node()) {
			nodes = append(nodes, name)
		}
	}
	return nodes
}

func (nc *nodeCache) CreateNodeInfoExt(nodename string) *NodeInfoExt {
	nc.Lock()

	if info, ok := nc.cache[nodename]; ok {
		info = info.Clone()
		nc.Unlock()

		score, err := CalcPrioritizeNodeScore(info)
		if err != nil {
			return nil
		}
		return &NodeInfoExt{info, score}
	} else {
		nc.Unlock()
		return nil
	}
}

// PrioritizeNodes simulates scheduler calculates the score, return HostPriorityList
func (nc *nodeCache) PrioritizeNodes() (schedulerapi.HostPriorityList, error) {
	nc.Lock()
	defer nc.Unlock()

	nodes := make([]*v1.Node, 0, len(nc.cache))
	for _, info := range nc.cache {
		if node := info.Node(); workNode(node) {
			nodes = append(nodes, node)
		}
	}
	return schedulercore.PrioritizeNodes(fakePod, nc.cache, nil, priorityConfig, nodes, nil)
}

// CalcPrioritizeNodeScore simulates scheduler calculates the score.
func CalcPrioritizeNodeScore(info *nodeinfo.NodeInfo) (int, error) {
	n := info.Node()
	m := map[string]*nodeinfo.NodeInfo{n.Name: info}
	nodes := []*v1.Node{n}

	hostPriorityList, err := schedulercore.PrioritizeNodes(fakePod, m, nil, priorityConfig, nodes, nil)
	if err != nil {
		klog.Errorf("calc node %v score error: %v", n.Name)
		return 0, err
	}
	return hostPriorityList[0].Score, nil
}
