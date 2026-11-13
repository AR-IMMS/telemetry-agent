// Package processowner starts one managed process with OS-native containment.
// It accepts an executable and non-secret arguments, then returns only after
// the process and any contained descendants have exited. It does not restart
// processes or own service-manager registration.
package processowner
