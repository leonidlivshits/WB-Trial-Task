package httpapi

const (
	codeInvalidArgument = "INVALID_ARGUMENT"
	codeUnauthorized    = "UNAUTHORIZED"
	codeConflict        = "CONFLICT"
	codeNotFound        = "NOT_FOUND"
	codeUnavailable     = "UNAVAILABLE"
)

const (
	msgTopUnavailable      = "top is unavailable"
	msgStopListUnavailable = "stop-list is unavailable"
	msgMetricsUnavailable  = "metrics are unavailable"
	msgOpenAPIUnavailable  = "openapi spec is unavailable"

	msgInvalidTopN      = "n must be in range [1, 100]"
	msgInvalidJSON      = "invalid json body"
	msgWordEmpty        = "word must not be empty"
	msgWordAlreadyExist = "word already exists"
	msgWordNotFound     = "word not found"
	msgInvalidAdminAuth = "invalid or missing X-Admin-Token"
)
