package balancer

import (
	"math/rand"
	"time"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	coretyped "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
)

// not need workqueue
type Balancer struct {
	clientset     kubernetes.Interface
	recyclePeriod time.Duration
	ratio         float64

	nodeCache       *nodeCache
	informerFactory informers.SharedInformerFactory
	eventRecorder   record.EventRecorder
}

func NewBalancer(clientset kubernetes.Interface, recyclePeriod time.Duration, ratio float64) *Balancer {
	nodeCache := newNodeCache()

	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	informerFactory.Core().V1().Pods().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Pod:
					return assignedPod(t)
				case cache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1.Pod); ok {
						return assignedPod(pod)
					}
					klog.Errorf("unable to convert object %T to *v1.Pod", obj)
					return false
				default:
					klog.Errorf("unable to handle object in %T", obj)
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    nodeCache.AddPod,
				UpdateFunc: nodeCache.UpdatePod,
				DeleteFunc: nodeCache.DeletePod,
			},
		},
	)
	informerFactory.Core().V1().Nodes().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    nodeCache.AddNode,
			UpdateFunc: nodeCache.UpdateNode,
			DeleteFunc: nodeCache.DeleteNode,
		},
	)

	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&coretyped.EventSinkImpl{Interface: clientset.CoreV1().Events("")})

	return &Balancer{
		clientset:     clientset,
		recyclePeriod: recyclePeriod,
		ratio:         ratio,

		nodeCache:       nodeCache,
		informerFactory: informerFactory,
		eventRecorder:   eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "resbalancer"}),
	}
}

func (b *Balancer) Run(stopCh <-chan struct{}) {
	b.informerFactory.Start(stopCh)
	b.informerFactory.WaitForCacheSync(stopCh)

	klog.Info("load all pods for init nodecache")
	if err := b.loadAllPods(); err != nil {
		klog.Errorf("loadAllPods error: %v", err)
	}

	wait.Until(func() {
		klog.V(1).Info("start balance now...")

		hostGSL, err := b.PrioritizeNodes()
		if err != nil {
			klog.Errorf("PrioritizeNodes error: %v", err)
			return
		}
		klog.V(4).Infof("hostGSL: %v", hostGSL)

		b.BalanceNodeRes(hostGSL)
	}, b.recyclePeriod, stopCh)
}

// We assume deleted pods schedule to outRightHosts and AddPod on them.
// When we RemovePod/AddPod it wil recalculate the score.
// STEP 1: resPressureOutLeftHostsMap delete pod one-by-one until the node score bigger than left.
// STEP 2: leftMeanHost rand delete pod one-by-one until outRightHostsMap.resFull is true.
func (b *Balancer) BalanceNodeRes(hostGSL *HostGSL) {
	resPressureHost := b.nodeCache.GetResPressureNode()
	outLeftHost, left := hostGSL.outLeftXsd(b.ratio)
	outRightHost, right := hostGSL.outRightXsd(b.ratio)
	klog.Infof("resPressureHost: %v", resPressureHost)
	klog.Infof("outLeftHost: %v, left: %v", outLeftHost, left)
	klog.Infof("outRightHost: %v, right: %v", outRightHost, right)

	// init resPressureOutLeftHostsMap, outRightHostsMap
	resPressureOutLeftHostsMap := make(nodeInfoExtHosts, len(resPressureHost)+len(outLeftHost))
	outRightHostsMap := make(nodeInfoExtHosts, len(outRightHost))
	for _, name := range resPressureHost {
		if info := b.nodeCache.CreateNodeInfoExt(name); info != nil {
			resPressureOutLeftHostsMap[name] = info
		}
	}
	for _, name := range outLeftHost {
		if info := b.nodeCache.CreateNodeInfoExt(name); info != nil {
			resPressureOutLeftHostsMap[name] = info
		}
	}
	for _, name := range outRightHost {
		if info := b.nodeCache.CreateNodeInfoExt(name); info != nil {
			outRightHostsMap[name] = info
		}
	}

	// STEP 1: resPressureOutLeftHostsMap delete pod one-by-one until the node score bigger than left
	for name, info := range resPressureOutLeftHostsMap {
		for _, pod := range info.Pods() {
			if !canDeletePod(pod) {
				continue
			}

			if err := b.clientset.CoreV1().Pods(pod.Namespace).Delete(pod.Name, nil); err != nil && !apierrors.IsNotFound(err) {
				b.eventRecorder.Eventf(pod, v1.EventTypeWarning, "FailedDeletePod", "delete pod %v error: %v", pod.Name, err)
				klog.Errorf("delete pod %v error: %v", pod.Name, err)
				continue
			}

			b.eventRecorder.Eventf(pod, v1.EventTypeNormal, "SuccessDeletePod", "delete pod %v success on node %v", pod.Name, name)
			klog.Infof("delete pod %v success on node %v", pod.Name, name)
			info.RemovePod(pod)
			outRightHostsMap.AddPod(pod, right) // assume schedule to outRightHostsMap and recalculate their score
			if info.GetScore() >= left {
				break
			}
		}
	}

	// STEP 2: leftMeanHost rand delete pod one-by-one until outRightHostsMap.resFull is true
	var leftMeanHost []string
	leftMeanHostsMap := nodeInfoExtHosts{}
	for !outRightHostsMap.isResFull(right) {
		rand.Seed(time.Now().UnixNano())

		if leftMeanHost == nil {
			// get once
			leftMeanHost = hostGSL.leftMean()
		}
		if len(leftMeanHost) == 0 {
			return
		}
		randIndex := rand.Intn(len(leftMeanHost))
		randNode := leftMeanHost[randIndex]
		info, ok := leftMeanHostsMap[randNode]
		if !ok {
			if info = b.nodeCache.CreateNodeInfoExt(randNode); len(info.Pods()) > 0 {
				leftMeanHostsMap[randNode] = info
			} else {
				// delete randIndex node
				leftMeanHost = append(leftMeanHost[:randIndex], leftMeanHost[randIndex+1:]...)
				continue
			}
		}
		if len(info.Pods()) == 0 {
			// delete randIndex node
			leftMeanHost = append(leftMeanHost[:randIndex], leftMeanHost[randIndex+1:]...)
			delete(leftMeanHostsMap, randNode)
			continue
		}
		pod := info.Pods()[rand.Intn(len(info.Pods()))]
		info.RemovePod(pod) // ensure not get forever

		if !canDeletePod(pod) {
			continue
		}

		if err := b.clientset.CoreV1().Pods(pod.Namespace).Delete(pod.Name, nil); err != nil && !apierrors.IsNotFound(err) {
			b.eventRecorder.Eventf(pod, v1.EventTypeWarning, "FailedDeletePod", "delete pod %v error: %v", pod.Name, err)
			klog.Errorf("delete pod %v error: %v", pod.Name, err)
			continue
		}

		b.eventRecorder.Eventf(pod, v1.EventTypeNormal, "SuccessDeletePod", "delete pod %v success on node %v", pod.Name, randNode)
		klog.Infof("delete pod %v success on node %v", pod.Name, randNode)
		outRightHostsMap.AddPod(pod, right) // assume schedule to outRightHostsMap and recalculate their score
	}
}

func (b *Balancer) PrioritizeNodes() (*HostGSL, error) {
	hostPriorityList, err := b.nodeCache.PrioritizeNodes()
	if err != nil {
		return nil, err
	}
	gsl := &HostGSL{HostPriorityList: hostPriorityList}
	gsl.Calc()
	return gsl, nil
}

func (b *Balancer) loadAllPods() error {
	pods, err := b.informerFactory.Core().V1().Pods().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if assignedPod(pod) {
			b.nodeCache.NewNode(pod.Spec.NodeName)
			b.nodeCache.AddPod(pod)
		}
	}
	return nil
}

type nodeInfoExtHosts map[string]*NodeInfoExt

func (m nodeInfoExtHosts) AddPod(pod *v1.Pod, right float64) {
	for _, info := range m {
		if info.GetScore() > right {
			info.AddPod(pod) // recalculate score
		}
	}
}

func (m nodeInfoExtHosts) isResFull(right float64) bool {
	for _, info := range m {
		if info.GetScore() > right {
			return false
		}
	}
	return true
}
