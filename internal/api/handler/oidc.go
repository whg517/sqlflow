package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// OIDCHandler handles OIDC authentication requests.
type OIDCHandler struct {
	oidcSvc *service.OIDCService
}

// NewOIDCHandler creates a new OIDCHandler.
func NewOIDCHandler(oidcSvc *service.OIDCService) *OIDCHandler {
	return &OIDCHandler{oidcSvc: oidcSvc}
}

// oidcLoginResponse is the response for the login initiation endpoint.
type oidcLoginResponse struct {
	AuthURL      string `json:"auth_url"`
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier"`
}

// oidcCallbackResponse is the response after successful OIDC callback.
type oidcCallbackResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

// oidcProviderInfo represents a public-facing provider info (no secrets).
type oidcProviderInfo struct {
	Name string `json:"name"`
}

// oidcProvidersResponse is the response for listing available providers.
type oidcProvidersResponse struct {
	Providers []oidcProviderInfo `json:"providers"`
}

// Login handles GET /api/auth/oidc/:provider — returns the authorization URL with PKCE.
// Login godoc
// @Summary OIDC 登录跳转
// @Description 生成 OIDC Authorization Code Flow + PKCE 授权 URL
// @Tags 认证
// @Param provider path string true "OIDC 提供者名称"
// @Param redirect_uri query string false "回调地址"
// @Success 200 {object} resp.SuccessResponse "授权链接"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 404 {object} resp.ErrorResponse "提供者不存在"
// @Router /api/auth/oidc/{provider} [get]
func (h *OIDCHandler) Login(c echo.Context) error {
	providerName := c.Param("provider")
	if providerName == "" {
		return resp.BadRequest(c, "缺少 provider 参数")
	}

	redirectURL := c.QueryParam("redirect_uri")
	if redirectURL == "" {
		return resp.BadRequest(c, "缺少 redirect_uri 参数")
	}

	params, err := h.oidcSvc.AuthURL(c.Request().Context(), providerName, redirectURL)
	if err != nil {
		if err == service.ErrOIDCProviderNotFound {
			return resp.NotFound(c, "OIDC 提供者不存在: "+providerName)
		}
		if err == service.ErrOIDCProviderDisabled {
			return resp.BadRequest(c, "OIDC 提供者已禁用: "+providerName)
		}
		return resp.InternalError(c, "获取 OIDC 授权链接失败")
	}

	return resp.OK(c, oidcLoginResponse{
		AuthURL:      params.AuthURL,
		State:        params.State,
		CodeVerifier: params.CodeVerifier,
	})
}

// Callback handles GET /api/auth/oidc/:provider/callback — processes the OIDC callback.
// Callback godoc
// @Summary OIDC 回调处理
// @Description 处理 OIDC 授权回调，交换 token 并登录
// @Tags 认证
// @Param provider path string true "OIDC 提供者名称"
// @Param code query string true "授权码"
// @Param state query string true "状态参数"
// @Param code_verifier query string true "PKCE code verifier"
// @Param redirect_uri query string false "回调地址"
// @Success 200 {object} resp.SuccessResponse "登录成功"
// @Failure 400 {object} resp.ErrorResponse "参数错误"
// @Failure 401 {object} resp.ErrorResponse "认证失败"
// @Router /api/auth/oidc/{provider}/callback [get]
func (h *OIDCHandler) Callback(c echo.Context) error {
	providerName := c.Param("provider")
	if providerName == "" {
		return resp.BadRequest(c, "缺少 provider 参数")
	}

	code := c.QueryParam("code")
	if code == "" {
		return resp.BadRequest(c, "缺少授权码参数 code")
	}

	_ = c.QueryParam("state") // Validated client-side; server stores in session/Redis for production

	codeVerifier := c.QueryParam("code_verifier")
	if codeVerifier == "" {
		return resp.BadRequest(c, "缺少 PKCE code_verifier 参数")
	}

	redirectURL := c.QueryParam("redirect_uri")
	if redirectURL == "" {
		return resp.BadRequest(c, "缺少 redirect_uri 参数")
	}

	token, user, err := h.oidcSvc.HandleCallback(c.Request().Context(), providerName, code, codeVerifier, redirectURL)
	if err != nil {
		if err == service.ErrOIDCProviderNotFound {
			return resp.NotFound(c, "OIDC 提供者不存在: "+providerName)
		}
		return resp.Unauthorized(c, "OIDC 登录失败: "+err.Error())
	}

	return resp.OK(c, oidcCallbackResponse{
		Token: token,
		User: userResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// Providers handles GET /api/auth/providers — lists available OIDC providers.
// Providers godoc
// @Summary 获取可用 OIDC 提供者列表
// @Description 返回所有已启用的 OIDC 身份提供者（不含密钥）
// @Tags 认证
// @Produce json
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /api/auth/providers [get]
func (h *OIDCHandler) Providers(c echo.Context) error {
	providers := h.oidcSvc.ListProviders()

	list := make([]oidcProviderInfo, 0, len(providers))
	for _, p := range providers {
		list = append(list, oidcProviderInfo{Name: p.Name})
	}

	return c.JSON(http.StatusOK, oidcProvidersResponse{Providers: list})
}
