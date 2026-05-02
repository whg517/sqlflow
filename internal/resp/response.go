package resp

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// SuccessResponse is the standard success response envelope.
type SuccessResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ErrorResponse is the standard error response envelope.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PageResponse wraps paginated data with metadata.
type PageResponse struct {
	Code     int         `json:"code"`
	Message  string      `json:"message"`
	Data     interface{} `json:"data"`
	Page     int64       `json:"page"`
	PageSize int64       `json:"page_size"`
	Total    int64       `json:"total"`
}

// OK returns a 200 success response.
func OK(c echo.Context, data interface{}) error {
	return c.JSON(http.StatusOK, SuccessResponse{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// OKWithMessage returns a 200 success response with a custom message.
func OKWithMessage(c echo.Context, message string, data interface{}) error {
	return c.JSON(http.StatusOK, SuccessResponse{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// Created returns a 201 success response.
func Created(c echo.Context, data interface{}) error {
	return c.JSON(http.StatusCreated, SuccessResponse{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

// OKPage returns a 200 paginated response.
func OKPage(c echo.Context, data interface{}, page, pageSize, total int64) error {
	return c.JSON(http.StatusOK, PageResponse{
		Code:     0,
		Message:  "ok",
		Data:     data,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

// BadRequest returns a 400 error response.
func BadRequest(c echo.Context, msg string) error {
	return c.JSON(http.StatusBadRequest, ErrorResponse{
		Code:    400,
		Message: msg,
	})
}

// Unauthorized returns a 401 error response.
func Unauthorized(c echo.Context, msg string) error {
	return c.JSON(http.StatusUnauthorized, ErrorResponse{
		Code:    401,
		Message: msg,
	})
}

// Forbidden returns a 403 error response.
func Forbidden(c echo.Context, msg string) error {
	return c.JSON(http.StatusForbidden, ErrorResponse{
		Code:    403,
		Message: msg,
	})
}

// NotFound returns a 404 error response.
func NotFound(c echo.Context, msg string) error {
	return c.JSON(http.StatusNotFound, ErrorResponse{
		Code:    404,
		Message: msg,
	})
}

// InternalError returns a 500 error response.
func InternalError(c echo.Context, msg string) error {
	return c.JSON(http.StatusInternalServerError, ErrorResponse{
		Code:    500,
		Message: msg,
	})
}
