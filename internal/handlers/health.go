package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck is the simplest possible endpoint.
// Its only job is to confirm the server is running and reachable.
// In production systems, load balancers and monitoring tools
// (like AWS ALB or Kubernetes) ping this endpoint every few seconds
// to decide whether to route traffic to this instance.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "propvest-api",
	})
}