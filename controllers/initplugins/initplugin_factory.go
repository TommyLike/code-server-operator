package initplugins

import (
	"fmt"
	"github.com/tommylike/code-server-operator/controllers/initplugins/git"
	"github.com/tommylike/code-server-operator/controllers/initplugins/interface"
	"sync"
)

func init() {
	RegisterPlugins(git.GetName(), git.Create)
}

var pluginMutex sync.Mutex

var pluginBuilders = map[string]PluginBuilder{}

type PluginBuilder func(c _interface.PluginClients, parameters []string, baseDir string) _interface.PluginInterface

func RegisterPlugins(name string, pc func(c _interface.PluginClients, parameters []string, baseDir string) _interface.PluginInterface) {
	pluginMutex.Lock()
	defer pluginMutex.Unlock()
	pluginBuilders[name] = pc
}

// Create Plugin via name and parameters
func CreatePlugin(client _interface.PluginClients, name string, parameters []string, baseDir string) (_interface.PluginInterface, error) {
	pluginMutex.Lock()
	defer pluginMutex.Unlock()
	pb, found := pluginBuilders[name]
	if !found {
		return nil, fmt.Errorf("can't find init plugin %s in controllers", name)
	}
	initPlugins := pb(client, parameters, baseDir)
	return initPlugins, nil
}
