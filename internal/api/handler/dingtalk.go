package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// DingTalkHandler handles DingTalk OAuth login requests.
type DingTalkHandler struct {
	dingSvc *service.DingTalkOAuthService
}

// NewDingTalkHandler creates a new DingTalkHandler.
func NewDingTalkHandler(dingSvc *service.DingTalkOAuthService) *DingTalkHandler {
	return &DingTalkHandler{dingSvc: dingSvc}
}

// dingtalkLoginResponse is the response for the login endpoint.
type dingtalkLoginResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

// dingtalkCallbackResponse is the response after successful OAuth callback.
type dingtalkCallbackResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

// Login handles GET /api/v1/auth/dingtalk/login — returns the authorization URL.
func (h *DingTalkHandler) Login(c echo.Context) error {
	authURL, state, err := h.dingSvc.AuthURL()
	if err != nil {
		if err == service.ErrDingTalkDisabled {
			return resp.BadRequest(c, "钉钉登录未启用")
		}
		return resp.InternalError(c, "获取钉钉授权链接失败")
	}

	return resp.OK(c, dingtalkLoginResponse{
		AuthURL: authURL,
		State:   state,
	})
}

// Callback handles GET /api/v1/auth/dingtalk/callback — processes the OAuth callback.
func (h *DingTalkHandler) Callback(c echo.Context) error {
	code := c.QueryParam("code")
	if code == "" {
		return resp.BadRequest(c, "缺少授权码参数 code")
	}

	state := c.QueryParam("state")
	_ = state // In production, validate state against session-stored value for CSRF protection.

	token, user, err := h.dingSvc.HandleCallback(c.Request().Context(), code)
	if err != nil {
		if err == service.ErrDingTalkDisabled {
			return resp.BadRequest(c, "钉钉登录未启用")
		}
		return resp.Unauthorized(c, "钉钉登录失败: "+err.Error())
	}

	return resp.OK(c, dingtalkCallbackResponse{
		Token: token,
		User: userResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// Enabled returns whether DingTalk OAuth is configured.
func (h *DingTalkHandler) Enabled(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"enabled": h.dingSvc.Enabled()})
}
