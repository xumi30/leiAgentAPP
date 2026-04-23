package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"proxy-lb/internal/logging"
)

func NewRouter(service *Service) *gin.Engine {
	gin.DefaultWriter = logging.GinWriter()
	gin.DefaultErrorWriter = logging.GinWriter()

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		requestID := logging.NextRequestID()
		ctx := logging.ContextWithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Writer.Header().Set("X-Request-Id", requestID)
		start := time.Now()
		if c.Request.Method == http.MethodGet && c.FullPath() == "/v1/models" {
			logging.DebugfCtx(ctx, "request start method=%s path=%s remote=%s", c.Request.Method, c.FullPath(), c.ClientIP())
		} else {
			logging.InfofCtx(ctx, "request start method=%s path=%s remote=%s", c.Request.Method, c.FullPath(), c.ClientIP())
		}
		c.Next()
		if c.Request.Method == http.MethodGet && c.FullPath() == "/v1/models" {
			logging.DebugfCtx(ctx, "request done method=%s path=%s status=%d duration=%s", c.Request.Method, c.FullPath(), c.Writer.Status(), time.Since(start).Round(time.Millisecond))
		} else {
			logging.InfofCtx(ctx, "request done method=%s path=%s status=%d duration=%s", c.Request.Method, c.FullPath(), c.Writer.Status(), time.Since(start).Round(time.Millisecond))
		}
	})

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, healthResponse{
			OK:            true,
			DefaultModel:  service.cfg.Server.DefaultModel,
			ConfiguredIDs: configuredModelNames(service.models),
		})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":          "proxy-lb",
			"status":        "ok",
			"default_model": service.cfg.Server.DefaultModel,
			"models":        configuredModelNames(service.models),
			"endpoints": []string{
				"/healthz",
				"/readyz",
				"/auth/register",
				"/auth/login",
				"/auth/me",
				"/auth/tokens/issue",
				"/v1/models",
				"/v1/chat/completions",
			},
		})
	})

	router.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, gin.H{
			"ok":       true,
			"probes":   service.checkBackends(ctx),
			"models":   configuredModelNames(service.models),
			"defaults": service.cfg.Server.DefaultModel,
		})
	})

	router.POST("/auth/register", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Label    string `json:"label"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.WarnfCtx(c.Request.Context(), "register bad request: %v", err)
			writeJSONError(c.Writer, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		resp, err := service.auth.register(req.Username, req.Password, req.Label)
		if err != nil {
			logging.WarnfCtx(c.Request.Context(), "register failed username=%s err=%v", strings.TrimSpace(req.Username), err)
			writeJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		logging.InfofCtx(c.Request.Context(), "register succeeded username=%s", resp.Username)
		c.JSON(http.StatusOK, gin.H{
			"username": resp.Username,
			"token":    resp.Token,
		})
	})

	router.POST("/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Label    string `json:"label"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.WarnfCtx(c.Request.Context(), "login bad request: %v", err)
			writeJSONError(c.Writer, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		resp, err := service.auth.login(req.Username, req.Password, req.Label)
		if err != nil {
			logging.WarnfCtx(c.Request.Context(), "login failed username=%s err=%v", strings.TrimSpace(req.Username), err)
			writeJSONError(c.Writer, http.StatusUnauthorized, err.Error())
			return
		}
		logging.InfofCtx(c.Request.Context(), "login succeeded username=%s", resp.Username)
		c.JSON(http.StatusOK, gin.H{
			"username": resp.Username,
			"token":    resp.Token,
		})
	})

	protected := router.Group("/")
	protected.Use(authMiddleware(service))

	protected.GET("/auth/me", func(c *gin.Context) {
		principalAny, _ := c.Get("auth.principal")
		principal, _ := principalAny.(*authPrincipal)
		c.JSON(http.StatusOK, gin.H{
			"username":  principal.Username,
			"isIssued":  principal.IsIssued,
			"isStatic":  principal.IsStatic,
			"authToken": strings.TrimSpace(c.GetString("auth.token")) != "",
		})
	})

	protected.POST("/auth/tokens/issue", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Label    string `json:"label"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.WarnfCtx(c.Request.Context(), "issue token bad request: %v", err)
			writeJSONError(c.Writer, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		principalAny, _ := c.Get("auth.principal")
		principal, _ := principalAny.(*authPrincipal)
		username := strings.TrimSpace(req.Username)
		if principal != nil && !principal.IsStatic {
			username = principal.Username
		}
		resp, err := service.auth.issueToken(username, req.Label)
		if err != nil {
			logging.WarnfCtx(c.Request.Context(), "issue token failed username=%s err=%v", username, err)
			writeJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		logging.InfofCtx(c.Request.Context(), "issue token succeeded username=%s", resp.Username)
		c.JSON(http.StatusOK, gin.H{
			"username": resp.Username,
			"token":    resp.Token,
		})
	})

	protected.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, ginModelsResponse{
			Object: "list",
			Data:   service.listModels(),
		})
	})

	protected.POST("/v1/chat/completions", func(c *gin.Context) {
		var req ChatCompletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.WarnfCtx(c.Request.Context(), "chat completion bad request: %v", err)
			writeJSONError(c.Writer, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		result, status, err := service.handleChatCompletions(c.Request.Context(), &req)
		if err != nil {
			logging.WarnfCtx(c.Request.Context(), "chat completion failed status=%d err=%v", status, err)
			writeJSONError(c.Writer, status, err.Error())
			return
		}

		if result.Stream {
			streamOpenAIResponse(c.Writer, result)
			return
		}
		if req.Stream {
			streamSyntheticResponse(c.Writer, result.OpenAIResp)
			return
		}
		c.JSON(http.StatusOK, result.OpenAIResp)
	})

	return router
}

func authMiddleware(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		principal, err := service.authenticateBearerToken(token)
		if err != nil {
			logging.WarnfCtx(c.Request.Context(), "auth failed path=%s", c.FullPath())
			writeJSONError(c.Writer, http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		if c.FullPath() == "/v1/models" {
			logging.DebugfCtx(c.Request.Context(), "auth success user=%s static=%v path=%s", principal.Username, principal.IsStatic, c.FullPath())
		} else {
			logging.InfofCtx(c.Request.Context(), "auth success user=%s static=%v path=%s", principal.Username, principal.IsStatic, c.FullPath())
		}
		c.Set("auth.principal", principal)
		c.Set("auth.token", token)
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token == "" {
		token = strings.TrimSpace(c.GetHeader("X-API-Key"))
	}
	return token
}
