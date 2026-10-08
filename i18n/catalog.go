package i18n

import "embed"

// LocaleFS 保存所有服务共享的基础词典。
//
//go:embed locale/*.json
var LocaleFS embed.FS

const (
	Success            = "common.success"
	Failed             = "common.failed"
	CreateSuccess      = "common.createSuccess"
	CreateFailed       = "common.createFailed"
	UpdateSuccess      = "common.updateSuccess"
	UpdateFailed       = "common.updateFailed"
	DeleteSuccess      = "common.deleteSuccess"
	DeleteFailed       = "common.deleteFailed"
	TargetNotFound     = "common.targetNotFound"
	DatabaseError      = "common.databaseError"
	CacheError         = "common.cacheError"
	ConstraintError    = "common.constraintError"
	ValidationError    = "common.validationError"
	PermissionDenied   = "common.permissionDenied"
	ServiceUnavailable = "common.serviceUnavailable"
	ServiceBusy        = "common.serviceBusy"
)
