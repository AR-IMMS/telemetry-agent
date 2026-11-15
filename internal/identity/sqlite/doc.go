// Package sqlite adapts installation identity persistence to SQLite.
//
// It provides the Identity migration definition and installation repository; the
// composition boundary registers its migration in the global version order. This
// package does not own database lifecycle, identity generation policy, other Agent
// state, or secret storage.
package sqlite
