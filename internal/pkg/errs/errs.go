package errs

import "errors"

// Result 统一响应格式，字段对齐 Java 版 Result（non_null 序列化 → omitempty）。
type Result struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg,omitempty"`
	Data     any    `json:"data,omitempty"`
	Total    int64  `json:"total,omitempty"`
}

func OK(data ...any) Result {
	r := Result{Success: true}
	if len(data) > 0 {
		r.Data = data[0]
	}
	return r
}

func Fail(msg string) Result {
	return Result{Success: false, ErrorMsg: msg}
}

// 哨兵错误，service 层返回，handler 层映射为响应。
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInternal     = errors.New("internal error")
	ErrInvalidPhone = errors.New("invalid phone")
	ErrCodeMismatch = errors.New("code mismatch")

	ErrStockEmpty      = errors.New("stock empty")
	ErrDuplicatedOrder = errors.New("duplicated order")
)
