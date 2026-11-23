// Package docker adapts Docker Engine summaries and inspection to the provider-neutral Inventory contract.
//
// It reads explicit, bounded resource evidence and never collects secrets,
// environment variables, command lines, volume contents, or Integration config.
package docker
