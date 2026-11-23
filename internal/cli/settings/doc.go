// Package settings resolves CLI values from the currently approved sources.
// It binds existing flag values through pflag and Viper while keeping
// environment and preference-file loading disabled until their precedence and
// paths are approved; it does not own Agent desired state or secrets.
package settings
