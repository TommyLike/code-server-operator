package git

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/tommylike/code-server-operator/controllers/initplugins/interface"
	corev1 "k8s.io/api/core/v1"
)

const (
	DefaultImageUrl = "alpine/git:1.0.8"
)

type GitPlugin struct {
	Client        _interface.PluginClients
	Parameters    []string
	ImageUrl      string
	RepoUrl       string
	BaseDirectory string
}

func (p *GitPlugin) parseFlags(arguments []string) {
	flagSet := flag.NewFlagSet(GetName(), flag.ContinueOnError)
	flagSet.StringVar(&p.RepoUrl, "repourl", "", "The git repo for git plugin to clone, username and password should be provided if it's private repo, for instance: https://username:password@github.com/username/repository.git")
	if err := flagSet.Parse(arguments); err != nil {
		glog.Errorf("plugin %s flagset parse failed, err: %v", GetName(), err)
	}
	p.ImageUrl = DefaultImageUrl
	return
}

func Create(c _interface.PluginClients, parameters []string, baseDir string) _interface.PluginInterface {
	gitPlugin := GitPlugin{Client: c, BaseDirectory: baseDir}
	gitPlugin.parseFlags(parameters)
	return &gitPlugin
}

func (p *GitPlugin) GenerateInitContainerSpec() *corev1.Container {

	command := []string{"sh", "-c", fmt.Sprintf("cd %s && git clone %s", p.BaseDirectory, p.RepoUrl)}
	container := corev1.Container{
		Image:           p.ImageUrl,
		Name:            "init-git-clone",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         command,
	}
	return &container
}

func (p *GitPlugin) SetDefaultImage(imageUrl string) {
	p.ImageUrl = imageUrl
}

func (p *GitPlugin) SetWorkingDir(baseDir string) {
	p.BaseDirectory = baseDir
}

func GetName() string {
	return "git"
}
