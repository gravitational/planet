package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"
	log "github.com/sirupsen/logrus"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// setupResolver finds the kube-dns service address, and writes an environment file accordingly
func setupResolver(ctx context.Context, role agent.Role) error {
	client, err := cmd.GetKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	err = utils.Retry(ctx, math.MaxInt64, 1*time.Second, func() error {
		env, err := getEnvDNSAddresses(client, role)
		if err != nil {
			return trace.Wrap(err)
		}

		return trace.Wrap(writeEnvDNSAddresses(env))
	})
	return trace.Wrap(err)
}

func writeEnvDNSAddresses(env string) error {
	err := utils.SafeWriteFile(DNSEnvFile, []byte(env), constants.SharedReadMask)
	if err != nil {
		return trace.Wrap(err)
	}

	// restart kubelet, so it picks up the new DNS settings
	err = exec.Command("systemctl", "restart", "kube-kubelet").Run()
	if err != nil {
		//
		log.Errorf("Error restart kubelet: %v", err)
	}
	return nil
}

func getEnvDNSAddresses(client *kubernetes.Clientset, role agent.Role) (string, error) {
	// try and locate the kube-dns svc clusterIP
	svcMaster, err := client.CoreV1().Services(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}

	svcWorker, err := client.CoreV1().Services(metav1.NamespaceSystem).Get("kube-dns-worker", metav1.GetOptions{})
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}

	if svcMaster.Spec.ClusterIP == "" {
		return "", trace.BadParameter("service/kube-dns Spec.ClusterIP is empty")
	}
	if svcWorker.Spec.ClusterIP == "" {
		return "", trace.BadParameter("service/kube-dns-worker Spec.ClusterIP is empty")
	}

	// If we're a master server, only use the master servers as a resolver.
	// This is because, we don't know if the second worker service will have any pods after future scaling operations
	//
	// If we're a worker, query the workers coredns first, and master second
	// This guaranteess any retries will not be handled by the same node
	if role == agent.RoleMaster {
		return fmt.Sprintf("%v=\"%v\"\n", EnvDNSAddresses, svcMaster.Spec.ClusterIP), nil
	}
	return fmt.Sprintf("%v=\"%v,%v\"\n", EnvDNSAddresses, svcWorker.Spec.ClusterIP, svcMaster.Spec.ClusterIP), nil

}
