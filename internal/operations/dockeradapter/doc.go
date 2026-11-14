// Package dockeradapter provides the narrow local Docker Engine operations
// port used by Agent Operations. It inspects only restart eligibility and
// verification evidence and issues one restart request; it does not own
// business authorization, discovery, retries, or execution journaling.
package dockeradapter
