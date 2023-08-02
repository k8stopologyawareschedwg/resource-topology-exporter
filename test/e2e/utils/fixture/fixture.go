package fixture

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
)

var fxt Fixture
var initialized bool

type Fixture struct {
	Ctx     context.Context
	Cli     client.Client
	K8SCli  *kubernetes.Clientset
	TopoCli *topologyclientset.Clientset
	ApiExt  *apiextension.Clientset
	NS      *corev1.Namespace
}

func New() *Fixture {
	// important so we keep the same context across the suite run
	if initialized {
		return &fxt
	}
	fxt.Ctx = context.Background()
	cfg, err := config.GetConfig()
	if err != nil {
		klog.Exit(err.Error())
	}
	if err := initK8SClient(cfg); err != nil {
		klog.Exit(err.Error())
	}
	if err := initClient(cfg); err != nil {
		klog.Exit(err.Error())
	}
	if err := initTopologyClient(cfg); err != nil {
		klog.Exit(err.Error())
	}
	if err := initApiExtensionClient(cfg); err != nil {
		klog.Exit(err.Error())
	}
	initialized = true
	return &fxt
}

func initClient(cfg *rest.Config) error {
	var err error

	fxt.Cli, err = client.New(cfg, client.Options{})
	return err
}

func initK8SClient(cfg *rest.Config) error {
	var err error

	fxt.K8SCli, err = kubernetes.NewForConfig(cfg)
	return err
}

func initTopologyClient(cfg *rest.Config) error {
	var err error

	fxt.TopoCli, err = topologyclientset.NewForConfig(cfg)
	return err
}

func initApiExtensionClient(cfg *rest.Config) error {
	var err error

	fxt.ApiExt, err = apiextension.NewForConfig(cfg)
	return err
}

// CreateNamespace creates a namespace with the given prefix
// and return a cleanup function for the newly created namespace
func (fxt *Fixture) CreateNamespace(prefix string) (func() error, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "rte-e2e-testing-" + prefix + "-",
			Labels: map[string]string{
				"security.openshift.io/scc.podSecurityLabelSync": "false",
				"pod-security.kubernetes.io/audit":               "privileged",
				"pod-security.kubernetes.io/enforce":             "privileged",
				"pod-security.kubernetes.io/warn":                "privileged",
			},
		},
	}
	err := fxt.Cli.Create(fxt.Ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace %s; %w", ns.Name, err)
	}
	fxt.NS = ns
	cleanFunc := func() error {
		err = fxt.Cli.Delete(fxt.Ctx, ns)
		if err != nil {
			return fmt.Errorf("failed to delete namespace %s; %w", ns.Name, err)
		}
		return nil
	}
	return cleanFunc, nil
}
