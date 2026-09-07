package version

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.

// GetSingleVersion returns single version of sealer
func GetSingleVersion() string {
	return gitVersion
}

type Output struct {
	ChatVersion   Info           `json:"ChatVersion" yaml:"ChatVersion,omitempty"`
	ServerVersion *ServerVersion `json:"ServerVersion,omitempty" yaml:"ServerVersion,omitempty"`
}

type ServerVersion struct {
	ServerVersion string `json:"serverVersion,omitempty" yaml:"serverVersion,omitempty"`
	ClientVersion string `json:"clientVersion,omitempty" yaml:"clientVersion,omitempty"` //sdk core version
}
