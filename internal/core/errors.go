package core

import "fmt"

// ErrorCode 错误码
type ErrorCode string

const (
	ErrProviderUnavailable    ErrorCode = "CB_PROVIDER_UNAVAILABLE"
	ErrProviderAuthFailed     ErrorCode = "CB_PROVIDER_AUTH_FAILED"
	ErrProviderModelNotFound  ErrorCode = "CB_PROVIDER_MODEL_NOT_FOUND"
	ErrPatchInvalid           ErrorCode = "CB_PATCH_INVALID"
	ErrPatchOutOfScope        ErrorCode = "CB_PATCH_OUT_OF_SCOPE"
	ErrPatchApplyFailed       ErrorCode = "CB_PATCH_APPLY_FAILED"
	ErrSnapshotFailed         ErrorCode = "CB_SNAPSHOT_FAILED"
	ErrBackupFailed           ErrorCode = "CB_BACKUP_FAILED"
	ErrCommandTimeout         ErrorCode = "CB_COMMAND_TIMEOUT"
	ErrEncodingError          ErrorCode = "CB_ENCODING_ERROR"
	ErrForbiddenFileAccess    ErrorCode = "CB_FORBIDDEN_FILE_ACCESS"
	ErrHighRiskBlocked        ErrorCode = "CB_HIGH_RISK_BLOCKED"
	ErrConfigInvalid          ErrorCode = "CB_CONFIG_INVALID"
	ErrTaskAlreadyRunning     ErrorCode = "CB_TASK_ALREADY_RUNNING"
	ErrTaskRecoveryRequired   ErrorCode = "CB_TASK_RECOVERY_REQUIRED"
)

// BridgeError coding-bridge 错误
type BridgeError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *BridgeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *BridgeError) Unwrap() error {
	return e.Cause
}

// NewBridgeError 创建新错误
func NewBridgeError(code ErrorCode, msg string, cause error) *BridgeError {
	return &BridgeError{Code: code, Message: msg, Cause: cause}
}

// 常用错误工厂函数

func ErrProviderUnavailablef(msg string, args ...any) *BridgeError {
	return NewBridgeError(ErrProviderUnavailable, fmt.Sprintf(msg, args...), nil)
}

func ErrPatchInvalidf(msg string, args ...any) *BridgeError {
	return NewBridgeError(ErrPatchInvalid, fmt.Sprintf(msg, args...), nil)
}

func ErrPatchOutOfScopef(msg string, args ...any) *BridgeError {
	return NewBridgeError(ErrPatchOutOfScope, fmt.Sprintf(msg, args...), nil)
}

func ErrHighRiskBlockedf(msg string, args ...any) *BridgeError {
	return NewBridgeError(ErrHighRiskBlocked, fmt.Sprintf(msg, args...), nil)
}

func ErrForbiddenFileAccessf(msg string, args ...any) *BridgeError {
	return NewBridgeError(ErrForbiddenFileAccess, fmt.Sprintf(msg, args...), nil)
}
