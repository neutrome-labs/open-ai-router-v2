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

// HeadPlugins are plugins that are always executed before others
var HeadPlugins = [][2]string{
	{"models", ""},
}

// TailPlugins are plugins that are always executed after others
var TailPlugins = [][2]string{
	{"posthog", ""},
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
