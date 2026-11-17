// Package dependency presents existing Runtime dependency catalog operations.
//
// It receives catalog results through a narrow callback and writes the legacy
// text and exit-code contract. It does not own dependency policy, installation,
// persistence, or platform lifecycle behavior.
package dependency
