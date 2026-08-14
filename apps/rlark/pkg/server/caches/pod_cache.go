package caches

import (
	"sync"

	"k8s.io/client-go/tools/cache"

	"github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// PodCache caches values.
type PodCache struct {
	mutex       sync.RWMutex
	podInformer cache.SharedIndexInformer
	pods        map[string]*v1alpha1.Pod
	taskPods    map[string]*v1alpha1.Pod
}

// NewPodCache creates a new PodCache.
func NewPodCache(podInformer cache.SharedIndexInformer) *PodCache {
	c := &PodCache{
		podInformer: podInformer,
		pods:        make(map[string]*v1alpha1.Pod),
		taskPods:    make(map[string]*v1alpha1.Pod),
	}
	_, _ = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	})
	return c
}

func (c *PodCache) setIfNewer(pod *v1alpha1.Pod) {
	// set pod by podName
	if existPod, exists := c.pods[pod.Spec.PodName]; exists {
		if pod.ResourceVersion > existPod.ResourceVersion { // update 不会修改 CreationTimestamp
			c.pods[pod.Spec.PodName] = pod
		}
	} else {
		c.pods[pod.Spec.PodName] = pod
	}
	// set pod by taskName
	if existPod, exists := c.taskPods[pod.Spec.TaskName]; exists {
		if pod.ResourceVersion > existPod.ResourceVersion {
			c.taskPods[pod.Spec.TaskName] = pod
		}
	} else {
		c.taskPods[pod.Spec.TaskName] = pod
	}
}

func (c *PodCache) onAdd(obj interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	pod, ok := obj.(*v1alpha1.Pod)
	if !ok {
		return
	}
	c.setIfNewer(pod)
}

func (c *PodCache) onUpdate(oldObj, newObj interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	pod, ok := newObj.(*v1alpha1.Pod)
	if !ok {
		return
	}
	c.setIfNewer(pod)
}

func (c *PodCache) onDelete(obj interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	pod, ok := obj.(*v1alpha1.Pod)
	if !ok {
		return
	}
	if existPod, exists := c.pods[pod.Spec.PodName]; exists {
		if pod.UID == existPod.UID {
			delete(c.pods, pod.Spec.PodName)
		}
	}
	if existPod, exists := c.taskPods[pod.Spec.TaskName]; exists {
		if pod.UID == existPod.UID {
			delete(c.taskPods, pod.Spec.TaskName)
		}
	}
}

// GetPodByName returns the podByName.
func (c *PodCache) GetPodByName(podName string) (*v1alpha1.Pod, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	pod, exists := c.pods[podName]
	return pod, exists
}

// GetPodByTaskName returns the podByTaskName.
func (c *PodCache) GetPodByTaskName(taskName string) (*v1alpha1.Pod, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	pod, exists := c.taskPods[taskName]
	return pod, exists
}
