package _interface

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PluginClients struct {
	Client client.Client
}

// PluginInterface interface
type PluginInterface interface {
	GenerateInitContainerSpec() *corev1.Container
	SetDefaultImage(imageUrl string)
}
