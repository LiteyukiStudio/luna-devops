package transport

import "github.com/gin-gonic/gin"

// markLiveObservationResponse prevents intermediaries and clients from reusing
// a point-in-time upstream observation as the current state.
func MarkLiveObservationResponse(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
}
