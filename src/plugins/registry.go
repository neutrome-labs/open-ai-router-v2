package plugins

// Registry holds all available plugins
var Registry = map[string]Plugin{
	"posthog": &Posthog{},
	"models":  &Models{},
	"fuzz":    &Fuzz{},
	"zip":     &Zip{PreserveFirst: false, DisableCache: false},
	"zipc":    &Zip{PreserveFirst: true, DisableCache: false},
	"zips":    &Zip{PreserveFirst: false, DisableCache: true},
	"zipsc":   &Zip{PreserveFirst: true, DisableCache: true},
}

// MandatoryPlugins are plugins that are always executed
var MandatoryPlugins = [][2]string{
	{"posthog", ""},
	{"models", ""},
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
