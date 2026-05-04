package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetMangouAgentRouter(router *gin.Engine) {
	router.GET("/skills/mangou-newapi/SKILL.md", controller.MangouAgentSkill)

	publicRouter := router.Group("/v1/agents")
	publicRouter.Use(middleware.RouteTag("relay"))
	{
		publicRouter.POST("/register/email-code", middleware.EmailVerificationRateLimit(), controller.MangouAgentSendEmailCode)
		publicRouter.POST("/register", middleware.CriticalRateLimit(), controller.MangouAgentRegister)
	}

	agentRouter := router.Group("/v1/agent")
	agentRouter.Use(middleware.RouteTag("relay"))
	agentRouter.Use(middleware.TokenAuth())
	{
		agentRouter.GET("/auth/check", controller.MangouAgentAuthCheck)
		agentRouter.POST("/tasks", controller.MangouAgentSubmitTask)
		agentRouter.GET("/tasks", controller.MangouAgentListTasks)
		agentRouter.GET("/tasks/:task_id", controller.MangouAgentGetTask)
	}
}
