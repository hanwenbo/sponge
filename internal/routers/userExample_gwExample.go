package routers

import (
	serverNameExampleV1 "github.com/zhufuyi/sponge/api/serverNameExample/v1"
	"github.com/zhufuyi/sponge/internal/service"

	"github.com/zhufuyi/sponge/pkg/logger"

	"github.com/gin-gonic/gin"
)

// nolint
func init() {
	rootRouterFns = append(rootRouterFns, func(r *gin.Engine) {
		userExampleRouter_gwExample(r, service.NewUserExampleServiceClient())
	})
}

func userExampleRouter_gwExample(r *gin.Engine, iService serverNameExampleV1.UserExampleServiceLogicer) { //nolint
	serverNameExampleV1.RegisterUserExampleServiceRouter(r, iService,
		serverNameExampleV1.WithUserExampleServiceRPCResponse(),
		serverNameExampleV1.WithUserExampleServiceLogger(logger.Get()),
	)
}