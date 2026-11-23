// Package migrate provides the technical coordinator for ordered SQLite schema
// migrations. It validates and runs owner-supplied migration functions while
// leaving SQL ownership and transaction boundaries with each migration owner.
// It imports no Agent business context and does not open or configure databases.
package migrate
