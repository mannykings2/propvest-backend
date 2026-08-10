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

// ReadyCheck is a readiness probe for orchestrators like Kubernetes.
// Unlike HealthCheck (liveness), this checks if the service is ready to serve traffic.
// It verifies dependencies like database connectivity.
//
// In production, this is used by:
//   - Kubernetes readiness probes (decides if pod should receive traffic)
//   - Load balancers (decides if instance is ready for requests)
//   - Deployment systems (waits for this before marking deployment complete)
//
// TODO: Add actual dependency checks (database, cache, message queue)
func ReadyCheck(c *gin.Context) {
	// TODO: Check database connection
	// TODO: Check cache connection
	// TODO: Check message queue connection
	
	// For now, return ready if the server is running
	response.Success(c, gin.H{
		"status":  "ready",
		"service": "propvest-api",
		"version": "1.0.0",
	})
}
