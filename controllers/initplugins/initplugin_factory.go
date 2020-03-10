package initplugins

import (
	"fmt"
	"github.com/tommylike/code-server-operator/controllers/initplugins/git"
	"github.com/tommylike/code-server-operator/controllers/initplugins/interface"
	"sync"
)

func init() {
	RegisterPlugins(git.GetName(), git.Create, false)
}

var pluginMutex sync.Mutex

var pluginsHolder = map[string]PluginCollections{}

type PluginBuilder func(c _interface.PluginClients, parameters []string, baseDir string) _interface.PluginInterface

type PluginCollections struct {
	BuildFunc          PluginBuilder
	PerformWhenRebuild bool
}

func RegisterPlugins(name string, pc func(c _interface.PluginClients, parameters []string, baseDir string) _interface.PluginInterface, performwhenrebuild bool) {
	pluginMutex.Lock()
	defer pluginMutex.Unlock()
	pluginsHolder[name] = PluginCollections{
		BuildFunc:          pc,
		PerformWhenRebuild: performwhenrebuild,
	}
}

// Create Plugin via name and parameters
func CreatePlugin(client _interface.PluginClients, name string, parameters []string, baseDir string, newVolume bool) (_interface.PluginInterface, error) {
	pluginMutex.Lock()
	defer pluginMutex.Unlock()
	pb, found := pluginsHolder[name]
	if !found {
		return nil, fmt.Errorf("can't find init plugin %s in controllers", name)
	}
	//skip plugin if it does not need execute when rebuild.
	if !newVolume {
		if !pb.PerformWhenRebuild {
			return nil, nil
		}
	}
	initPlugins := pb.BuildFunc(client, parameters, baseDir)
	return initPlugins, nil
}
