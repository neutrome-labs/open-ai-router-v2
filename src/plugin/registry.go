package plugin

// Registry holds all available plugins
var Registry = map[string]Plugin{
	/*"posthog":  &Posthog{},
	"models":   &Models{},
	"parallel": &Parallel{},
	"fuzz":     &Fuzz{},
	"stools":   &Stools{},*/
}

// GetPlugin returns a plugin by name
func GetPlugin(name string) (Plugin, bool) {
	p, ok := Registry[name]
	return p, ok
}

// RegisterPlugin registers a plugin
func RegisterPlugin(name string, p Plugin) {
	Registry[name] = p
}
