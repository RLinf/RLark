package gateway

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// --- Query parameter parsing ---

func (g *Gateway) parseListOptions(c *gin.Context) db.ListOptions {
	opts := db.ListOptions{
		Namespace: c.Query("namespace"),
	}

	// Pagination
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			opts.Limit = limit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			opts.Offset = offset
		}
	}

	// Ordering: comma-separated, e.g. "created_at desc,name"
	if orderStr := c.Query("order"); orderStr != "" {
		for _, field := range strings.Split(orderStr, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				opts.OrderBy = append(opts.OrderBy, field)
			}
		}
	}

	// Field selectors: comma-separated, e.g. "status.phase=Running,spec.agentType=Kubernetes"
	if fsStr := c.Query("fieldSelector"); fsStr != "" {
		for _, pair := range strings.Split(fsStr, ",") {
			if fs, err := parseFieldSelector(pair); err == nil {
				opts.FieldSelector = append(opts.FieldSelector, fs)
			}
		}
	}

	// Label selectors: comma-separated, e.g. "tenant=acme,env=prod"
	if lsStr := c.Query("labelSelector"); lsStr != "" {
		for _, pair := range strings.Split(lsStr, ",") {
			if ls, err := parseLabelSelector(pair); err == nil {
				opts.LabelSelector = append(opts.LabelSelector, ls)
			}
		}
	}

	return opts
}

// parseFieldSelector parses "path=value" or "path!=value" into a FieldSelector.
func parseFieldSelector(s string) (db.FieldSelector, error) {
	if strings.Contains(s, "!=") {
		parts := strings.SplitN(s, "!=", 2)
		return db.FieldSelector{Path: parts[0], Op: "!=", Value: parts[1]}, nil
	}
	if strings.Contains(s, "=") {
		parts := strings.SplitN(s, "=", 2)
		return db.FieldSelector{Path: parts[0], Op: "=", Value: parts[1]}, nil
	}
	return db.FieldSelector{}, strconv.ErrSyntax
}

// parseLabelSelector parses "key=value" or "key!=value" into a LabelSelector.
func parseLabelSelector(s string) (db.LabelSelector, error) {
	if strings.Contains(s, "!=") {
		parts := strings.SplitN(s, "!=", 2)
		return db.LabelSelector{Key: parts[0], Op: "!=", Value: parts[1]}, nil
	}
	if strings.Contains(s, "=") {
		parts := strings.SplitN(s, "=", 2)
		return db.LabelSelector{Key: parts[0], Op: "=", Value: parts[1]}, nil
	}
	return db.LabelSelector{}, strconv.ErrSyntax
}

// --- Read handler implementations (database or Kubernetes API-backed) ---

func (g *Gateway) handleList(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.dbClient != nil {
			if _, ok := g.stores[resource]; ok {
				g.handleListDB(c, resource)
				return
			}
		}
		g.handleListKube(c, resource)
	}
}

func (g *Gateway) handleGet(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.dbClient != nil {
			if _, ok := g.stores[resource]; ok {
				g.handleGetDB(c, resource)
				return
			}
		}
		g.handleGetKube(c, resource)
	}
}

// --- Database-backed read handlers ---

func (g *Gateway) handleListDB(c *gin.Context, resource string) {
	store, ok := g.stores[resource]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "resource not configured"})
		return
	}

	opts := g.parseListOptions(c)
	result, err := store.List(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (g *Gateway) handleGetDB(c *gin.Context, resource string) {
	store, ok := g.stores[resource]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "resource not configured"})
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")

	obj, err := store.Get(c.Request.Context(), namespace, name)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, obj)
}
