package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/external-dns/pkg/apis/externaldns"
	"sigs.k8s.io/external-dns/source"
)

func getLeaderElectionIdentity(cfg *externaldns.Config) string {
	if cfg.LeaderElectionIdentity != "" {
		return cfg.LeaderElectionIdentity
	}
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return fmt.Sprintf("external-dns-%d", time.Now().UnixNano())
}

func getLeaderElectionNamespace(cfg *externaldns.Config) (string, error) {
	if cfg.LeaderElectionNamespace != "" {
		return cfg.LeaderElectionNamespace, nil
	}

	// When running in a k8s cluster
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	// When running outside a k8s cluster
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}
	if kubeconfigPath == "" {
		return "", fmt.Errorf("cannot detect kubeconfig path")
	}

	if config, err := clientcmd.LoadFromFile(kubeconfigPath); err == nil {
		if current := config.CurrentContext; current != "" {
			if ctx, ok := config.Contexts[current]; ok && ctx.Namespace != "" {
				return ctx.Namespace, nil
			}
			return "", fmt.Errorf("no namespace defined in context %q of kubeconfig %q", current, kubeconfigPath)
		}
		return "", fmt.Errorf("no current context defined in kubeconfig %q", kubeconfigPath)
	} else {
		return "", fmt.Errorf("unable to load kubeconfig %q: %v", kubeconfigPath, err)
	}
}

func startLeaderElection(ctx context.Context, cfg *externaldns.Config, cliGen source.ClientGenerator, callbacks leaderelection.LeaderCallbacks) {
	clientset, err := cliGen.KubeClient()
	if err != nil {
		log.Fatalf("failed to create kube client for leader election: %v", err)
	}

	identity := getLeaderElectionIdentity(cfg)
	namespace, err := getLeaderElectionNamespace(cfg)
	if err != nil {
		log.Fatalf("failed to find leader election namespace: %v, please set --leader-election-namespace", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaderElectionLockName,
			Namespace: namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}

	lec := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   cfg.LeaderElectionLeaseDuration,
		RenewDeadline:   cfg.LeaderElectionRenewDeadline,
		RetryPeriod:     cfg.LeaderElectionRetryPeriod,
		Callbacks:       callbacks,
	}

	log.Debugf("attempting to acquire leader lease %s/%s with identity %s", namespace, cfg.LeaderElectionLockName, identity)
	leaderelection.RunOrDie(ctx, lec)
}
