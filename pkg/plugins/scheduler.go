package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type CustomSchedulerArgs struct {
	Mode string `json:"mode"`
}

type CustomScheduler struct {
	handle    framework.Handle
	scoreMode string
}

var _ framework.PreFilterPlugin = &CustomScheduler{}
var _ framework.ScorePlugin = &CustomScheduler{}

// Name is the name of the plugin used in Registry and configurations.
const (
	Name              string = "CustomScheduler"
	groupNameLabel    string = "podGroup"
	minAvailableLabel string = "minAvailable"
	leastMode         string = "Least"
	mostMode          string = "Most"
)

func (cs *CustomScheduler) Name() string {
	return Name
}

// New initializes and returns a new CustomScheduler plugin.
func New(obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	cs := CustomScheduler{}
	mode := leastMode
	if obj != nil {
		args := obj.(*runtime.Unknown)
		var csArgs CustomSchedulerArgs
		if err := json.Unmarshal(args.Raw, &csArgs); err != nil {
			fmt.Printf("Error unmarshal: %v\n", err)
		}
		mode = csArgs.Mode
		if mode != leastMode && mode != mostMode {
			return nil, fmt.Errorf("invalid mode, got %s", mode)
		}
	}
	cs.handle = h
	cs.scoreMode = mode
	log.Printf("Custom scheduler runs with the mode: %s.", mode)

	return &cs, nil
}

// filter the pod if the pod in group is less than minAvailable
func (cs *CustomScheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	log.Printf("Pod %s is in Prefilter phase.", pod.Name)
	newStatus := framework.NewStatus(framework.Success, "")

	// TODO
	// 1. extract the label of the pod
	// 2. retrieve the pod with the same group label
	// 3. justify if the pod can be scheduled
	podLabels := pod.ObjectMeta.Labels
	groupLabelValue := podLabels[groupNameLabel]
	minAvailableValue := podLabels[minAvailableLabel]
	log.Printf("groupLabel: %s", groupLabelValue)
	log.Printf("minAvailable: %s", minAvailableValue)

	minAvailable, err := strconv.Atoi(minAvailableValue)
	if err != nil {
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("Invalid minAvailable value: %v", err))
	}

	selector := labels.SelectorFromSet(map[string]string{groupNameLabel: groupLabelValue})
	pods, err := cs.handle.SharedInformerFactory().Core().V1().Pods().Lister().List(selector)
	if err != nil {
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("Failed to list pods: %v", err))
	}

	if len(pods) < minAvailable {
		log.Println("pods is not available")
		return nil, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Pod cannot be scheduled because the group '%s' has only %d pods, but needs %d", groupLabelValue, len(pods), minAvailable))
	}
	return nil, newStatus
}

// PreFilterExtensions returns a PreFilterExtensions interface if the plugin implements one.
func (cs *CustomScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}
func RemoveSubstring(s, sep string) string {
	if idx := strings.Index(s, sep); idx != -1 {
		return s[:idx]
	}
	return s
}

// Score invoked at the score extension point.
func (cs *CustomScheduler) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	log.Printf("Pod %s is in Score phase. Calculate the score of Node %s.", pod.Name, nodeName)

	// TODO
	// 1. retrieve the node allocatable memory
	// 2. return the score based on the scheduler mode
	nodeInfo, err := cs.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		log.Printf("Failed to get node info for node %s: %v", nodeName, err)
		return 0, framework.NewStatus(framework.Error, err.Error())
	}
	allocatableMemory := nodeInfo.Allocatable.Memory
	log.Printf("original allocatableMemory = %d", allocatableMemory)
	// var hasUsed int64 = 0
	// for _, podinNode := range nodeInfo.Pods {
	// 	// result := RemoveSubstring(podinNode.Pod.Spec.InitContainers[0].Resources.Limits.Memory().Format, "Mi")
	// 	hasUsed = podinNode.Pod.Spec.Containers[0].Resources.Limits.Memory().Value()
	// 	log.Printf("Pod %s Memory Resources Limit: %d", podinNode.Pod.ObjectMeta.Name, hasUsed)
	// }

	allocatableMemory -= nodeInfo.Requested.Memory
	log.Printf("node %s has been requested memory = %d", nodeName, nodeInfo.Requested.Memory)

	mode := cs.scoreMode
	var score int64 = 0
	log.Printf("score Mode: %s", mode)
	log.Printf("node %s now can be allocated memory: %d", nodeName, allocatableMemory)
	if mode == leastMode {
		score = 100000000000 / allocatableMemory
	} else {
		score = allocatableMemory
	}
	log.Printf("Node %s score is %d.", nodeName, score)
	log.Println()

	return score, nil
}

// ensure the scores are within the valid range
func (cs *CustomScheduler) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	// TODO
	// find the range of the current score and map to the valid range
	minScore := int64(math.MaxInt64)
	maxScore := int64(math.MinInt64)

	for _, nodeScore := range scores {
		if nodeScore.Score < minScore {
			minScore = nodeScore.Score
		}
		if nodeScore.Score > maxScore {
			maxScore = nodeScore.Score
		}
	}

	if minScore == maxScore {
		for i := range scores {
			scores[i].Score = framework.MaxNodeScore
		}
		return framework.NewStatus(framework.Success, "")
	}

	scoreRange := maxScore - minScore
	frameRange := framework.MaxNodeScore - framework.MinNodeScore
	for i := range scores {
		scores[i].Score = ((scores[i].Score-minScore)*frameRange)/scoreRange + framework.MinNodeScore
	}

	return framework.NewStatus(framework.Success, "")
	// return nil
}

// ScoreExtensions of the Score plugin.
func (cs *CustomScheduler) ScoreExtensions() framework.ScoreExtensions {
	return cs
}
