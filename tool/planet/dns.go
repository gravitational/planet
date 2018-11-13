package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"

	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// setupCoredns updates kubelet with the coredns service IP address
// for pod resolution
func setupCoredns(ctx context.Context) error {
	client, err := cmd.GetKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Looking for kube-dns service IP address")
	err = utils.Retry(ctx, math.MaxInt64, 1*time.Second, func() error {
		// try and locate the kube-dns svc clusterIP
		svc, err := client.CoreV1().Services(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
		if err != nil {
			log.Error("failed to retrieve kube-dns service: ", err)
			return trace.Wrap(err)
		}

		line := fmt.Sprintf("%v=\"%v\"\n", EnvDNSAddresses, svc.Spec.ClusterIP)
		log.Debug("Creating dns env: ", line)
		err = utils.SafeWriteFile(DNSEnvFile, []byte(line), constants.SharedReadMask)
		if err != nil {
			log.Warnf("Failed to write overlay environment %v: %v", DNSEnvFile, err)
			return trace.Wrap(err)
		}

		// restart kubelet, so it picks up the new DNS settings
		err = exec.Command("systemctl", "restart", "kube-kubelet").Run()
		if err != nil {
			log.Errorf("Error restart kubelet: %v", err)
		}

		return nil
	})
	return trace.Wrap(err)
}
