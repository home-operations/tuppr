//go:build e2e_hetzner

package e2ehetzner

import (
	"fmt"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newK8sClient(kubeconfig string) (client.WithWatch, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}
	if err := tupprv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding tuppr scheme: %w", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding apiextensions scheme: %w", err)
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("building rest config: %w", err)
	}

	c, err := client.NewWithWatch(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}
	return c, nil
}
