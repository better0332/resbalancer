package clientset

import (
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// GetClient auto obtain clientset
func GetClient(kubeconfigPath string) (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		klog.V(1).Info("get client config in k8s cluster")
	} else if err == rest.ErrNotInCluster {
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
			if kubeconfigPath == "" {
				kubeconfigPath = clientcmd.RecommendedHomeFile
			}
		}
		klog.V(1).Infof("use kubeconfigPath: %v", kubeconfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}
