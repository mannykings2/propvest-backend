package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/response"
)

// HealthCheck is the simplest possible endpoint.
// Its only job is to confirm the server is running and reachable.
// In production systems, load balancers and monitoring tools
// (like AWS ALB or Kubernetes) ping this endpoint every few seconds
// to decide whether to route traffic to this instance.
//
// This now uses the standard response format for consistency.
func HealthCheck(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "healthy",
		"service": "propvest-api",
		"version": "1.0.0",
	})
}