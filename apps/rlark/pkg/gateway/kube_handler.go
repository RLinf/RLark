package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	versioned "github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// resourceAccessor encapsulates Kubernetes client operations for a specific resource type.
type resourceAccessor struct {
	list   func(ctx context.Context, namespace string, opts metav1.ListOptions) (any, error)
	get    func(ctx context.Context, namespace, name string, opts metav1.GetOptions) (any, error)
	create func(ctx context.Context, namespace string, body json.RawMessage) (any, error)
	update func(ctx context.Context, namespace, name string, body json.RawMessage) (any, error)
	patch  func(ctx context.Context, namespace, name string, data []byte) (any, error)
	delete func(ctx context.Context, namespace, name string) error
}

// kubeClient wraps the typed CRUD operations for a Kubernetes resource type T.
// It centralizes the common create/update/patch/delete patterns so that each
// resource only needs to provide thin closures that call the typed client methods.
type kubeClient[T any] struct {
	doList   func(ctx context.Context, namespace string, opts metav1.ListOptions) (any, error)
	doGet    func(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*T, error)
	doCreate func(ctx context.Context, namespace string, obj *T, opts metav1.CreateOptions) (*T, error)
	doUpdate func(ctx context.Context, namespace string, obj *T, opts metav1.UpdateOptions) (*T, error)
	doPatch  func(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*T, error)
	doDelete func(ctx context.Context, namespace, name string, opts metav1.DeleteOptions) error
}

// accessor builds a resourceAccessor from the typed Kubernetes client,
// handling unmarshal, ResourceVersion propagation, and option wrapping.
func (c *kubeClient[T]) accessor() *resourceAccessor {
	return &resourceAccessor{
		list: c.doList,
		get: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (any, error) {
			return c.doGet(ctx, ns, name, opts)
		},
		create: func(ctx context.Context, ns string, body json.RawMessage) (any, error) {
			obj := new(T)
			if err := json.Unmarshal(body, obj); err != nil {
				return nil, err
			}
			return c.doCreate(ctx, ns, obj, metav1.CreateOptions{})
		},
		update: func(ctx context.Context, ns, name string, body json.RawMessage) (any, error) {
			current, err := c.doGet(ctx, ns, name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			obj := new(T)
			if err := json.Unmarshal(body, obj); err != nil {
				return nil, err
			}
			any(obj).(metav1.Object).SetResourceVersion(any(current).(metav1.Object).GetResourceVersion())
			return c.doUpdate(ctx, ns, obj, metav1.UpdateOptions{})
		},
		patch: func(ctx context.Context, ns, name string, data []byte) (any, error) {
			return c.doPatch(ctx, ns, name, types.MergePatchType, data, metav1.PatchOptions{})
		},
		delete: func(ctx context.Context, ns, name string) error {
			return c.doDelete(ctx, ns, name, metav1.DeleteOptions{})
		},
	}
}

// registerAccessors builds the resourceAccessor map for all supported resources.
func registerAccessors(client versioned.Interface) map[string]*resourceAccessor {
	return map[string]*resourceAccessor{
		"jobs": (&kubeClient[rlarkiov1alpha1.Job]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Jobs().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Job, opts metav1.CreateOptions) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Job, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Jobs().Delete(ctx, name, opts)
			},
		}).accessor(),

		"workflows": (&kubeClient[rlarkiov1alpha1.Workflow]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Workflows().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Workflow, opts metav1.CreateOptions) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Workflow, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Workflows().Delete(ctx, name, opts)
			},
		}).accessor(),

		"nodes": (&kubeClient[rlarkiov1alpha1.Node]{
			doList: func(ctx context.Context, ns string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Nodes(ns).List(ctx, opts)
			},
			doGet: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Node, opts metav1.CreateOptions) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Node, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, ns, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Nodes(ns).Delete(ctx, name, opts)
			},
		}).accessor(),

		"tasks": (&kubeClient[rlarkiov1alpha1.Task]{
			doList: func(ctx context.Context, ns string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Tasks(ns).List(ctx, opts)
			},
			doGet: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Task, opts metav1.CreateOptions) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Task, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, ns, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Tasks(ns).Delete(ctx, name, opts)
			},
		}).accessor(),

		"pods": (&kubeClient[rlarkiov1alpha1.Pod]{
			doList: func(ctx context.Context, ns string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Pods(ns).List(ctx, opts)
			},
			doGet: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Pod, opts metav1.CreateOptions) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Pod, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, ns, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Pods(ns).Delete(ctx, name, opts)
			},
		}).accessor(),

		"domains": (&kubeClient[rlarkiov1alpha1.Domain]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Domains().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Domain, opts metav1.CreateOptions) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Domain, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Domains().Delete(ctx, name, opts)
			},
		}).accessor(),
	}
}

// --- Generic Kubernetes client handlers using resourceAccessor ---

func (g *Gateway) handleListKube(c *gin.Context, resource string) {
	a, ok := g.accessors[resource]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
		return
	}

	ctx := c.Request.Context()
	namespace := c.Query("namespace")

	opts := metav1.ListOptions{}
	if limit := c.Query("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			opts.Limit = int64(n)
		}
	}
	if cont := c.Query("continue"); cont != "" {
		opts.Continue = cont
	}
	if fs := c.Query("fieldSelector"); fs != "" {
		opts.FieldSelector = fs
	}
	if ls := c.Query("labelSelector"); ls != "" {
		opts.LabelSelector = ls
	}

	result, err := a.list(ctx, namespace, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (g *Gateway) handleGetKube(c *gin.Context, resource string) {
	a, ok := g.accessors[resource]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
		return
	}

	ctx := c.Request.Context()
	name := c.Param("name")
	namespace := c.Query("namespace")

	result, err := a.get(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (g *Gateway) handleKubeCreate(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
			return
		}

		ctx := c.Request.Context()
		namespace := c.Query("namespace")

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := a.create(ctx, namespace, bodyBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, result)
	}
}

func (g *Gateway) handleKubeUpdate(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
			return
		}

		ctx := c.Request.Context()
		name := c.Param("name")
		namespace := c.Query("namespace")

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := a.update(ctx, namespace, name, bodyBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func (g *Gateway) handleKubePatch(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
			return
		}

		ctx := c.Request.Context()
		name := c.Param("name")
		namespace := c.Query("namespace")

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := a.patch(ctx, namespace, name, bodyBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func (g *Gateway) handleKubeDelete(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
			return
		}

		ctx := c.Request.Context()
		name := c.Param("name")
		namespace := c.Query("namespace")

		if err := a.delete(ctx, namespace, name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}
