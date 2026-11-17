// Package application owns proposal refresh, dismissal, and approval use cases.
// It accepts Monitoring recommendations and fresh Inventory evidence through
// ports, then requests Configuration-owned persistence; it does not discover
// resources, perform SQLite I/O, or activate the Collector.
package application
