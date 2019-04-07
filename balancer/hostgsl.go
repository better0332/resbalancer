package balancer

import (
	"fmt"

	"github.com/grd/stat"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"
)

type HostGSL struct {
	HostPriorityList schedulerapi.HostPriorityList

	mean float64
	sd   float64
}

func (gsl *HostGSL) Calc() {
	scores := make(stat.IntSlice, 0, len(gsl.HostPriorityList))
	for _, hp := range gsl.HostPriorityList {
		scores = append(scores, int64(hp.Score))
	}
	gsl.mean = stat.Mean(scores)
	gsl.sd = stat.SdMean(scores, gsl.mean)
}

func (gsl *HostGSL) Mean() float64 {
	return gsl.mean
}

func (gsl *HostGSL) Sd() float64 {
	return gsl.sd
}

func (gsl *HostGSL) outLeftXsd(ratio float64) (outLeftHost []string, left float64) {
	left = gsl.mean - ratio*gsl.sd
	if left <= 0 {
		return nil, 0
	}
	for _, hostPriority := range gsl.HostPriorityList {
		if float64(hostPriority.Score) < left {
			outLeftHost = append(outLeftHost, hostPriority.Host)
		}
	}
	return
}

func (gsl *HostGSL) leftMean() (leftMeanHost []string) {
	for _, hostPriority := range gsl.HostPriorityList {
		if float64(hostPriority.Score) < gsl.mean {
			leftMeanHost = append(leftMeanHost, hostPriority.Host)
		}
	}
	return
}

func (gsl *HostGSL) outRightXsd(ratio float64) (outRightHost []string, right float64) {
	right = gsl.mean + ratio*gsl.sd
	for _, hostPriority := range gsl.HostPriorityList {
		if float64(hostPriority.Score) > right {
			outRightHost = append(outRightHost, hostPriority.Host)
		}
	}
	return
}

func (gsl *HostGSL) String() string {
	return fmt.Sprintf("HostPriorityList: %v, mean: %v, sd: %v", gsl.HostPriorityList, gsl.mean, gsl.sd)
}
