package security

const (
	isolatedUIDBase  = 200000
	isolatedUIDRange = 400000
)

// IsolatedUIDForProcess assigns each live launcher PID a distinct unprivileged
// identity. Active Linux PIDs are unique, so concurrent requests cannot read
// each other's mode-0700 working directories even though they share a worker
// Pod.
func IsolatedUIDForProcess(pid int) int {
	if pid < 0 {
		pid = -pid
	}
	return isolatedUIDBase + pid%isolatedUIDRange
}
