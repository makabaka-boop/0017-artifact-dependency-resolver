package errcode

import "net/http"

// Code 是业务错误码。
type Code string

// 错误码常量；与 model 层状态对应的 HTTP 映射。
const (
	BadRequest    Code = "bad_request"
	NotFound      Code = "not_found"
	Conflict      Code = "conflict"
	Unprocessable Code = "unprocessable"
	Internal      Code = "internal"
	NotReady      Code = "not_ready"
)

// HTTPStatus 返回错误码对应的 HTTP 状态码。
func (c Code) HTTPStatus() int {
	switch c {
	case BadRequest:
		return http.StatusBadRequest
	case NotFound:
		return http.StatusNotFound
	case Conflict:
		return http.StatusConflict
	case Unprocessable:
		return http.StatusUnprocessableEntity
	case NotReady:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// APIError 是一个携带错误码、消息与明细的结构化错误。
type APIError struct {
	Code    Code     `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

// Error 实现 error 接口。
func (e *APIError) Error() string { return e.Message }

// New 构造一个 APIError。
func New(code Code, message string, details ...string) *APIError {
	return &APIError{Code: code, Message: message, Details: details}
}
