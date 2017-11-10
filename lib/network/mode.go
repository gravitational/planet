package network

import (
	"bytes"
	"net"
	"os/exec"

	tool "github.com/gravitational/planet/lib/network/ebtables"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// SetPromiscuousMode puts the specified interface iface into promiscuous mode
// and configures ebtable rules to eliminate duplicate packets.
func SetPromiscuousMode(ifaceName, podCidr string) error {
	log.Debugf("set promiscuous mode on %q", ifaceName)
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return trace.Wrap(err)
	}

	output, err := exec.Command(cmdIP, "link", "show", "dev", ifaceName).CombinedOutput()
	if err != nil || !bytes.Contains(output, promiscuousModeOn) {
		_, err := exec.Command(cmdIP, "link", "set", ifaceName, "promisc", "on").CombinedOutput()
		if err != nil {
			return trace.Wrap(err, "error setting promiscuous mode on %q", ifaceName)
		}
	}

	// get the IPv4 of the interface
	ipAddr, err := getIPv4Addr(*iface)
	if err != nil {
		return trace.Wrap(err)
	}

	// configure the ebtables rules to eliminate duplicate packets by best effort
	return syncEbtablesDedupRules(iface.HardwareAddr, ifaceName, podCidr, ipAddr)
}

func syncEbtablesDedupRules(macAddr net.HardwareAddr, ifaceName, podCidr string, gateway net.IP) error {
	if err := tool.FlushChain(tool.TableFilter, dedupChain); err != nil {
		log.Debugf("failed to flush deduplication chain: %v", err)
	}

	_, err := tool.GetVersion()
	if err != nil {
		return trace.Wrap(err, "failed to get ebtables version")
	}

	log.Debugf("filtering packets with ebtables on mac address %v, gateway %v and pod CIDR %v", macAddr, gateway, podCidr)
	_, err = tool.EnsureChain(tool.TableFilter, dedupChain)
	if err != nil {
		return trace.Wrap(err, "failed to create/update %q chain %q", tool.TableFilter, dedupChain)
	}

	_, err = tool.EnsureRule(tool.Append, tool.TableFilter, tool.ChainOutput, "-j", string(dedupChain))
	if err != nil {
		return trace.Wrap(err, "failed to ensure %v chain %v rule to jump to %v chain",
			tool.TableFilter, tool.ChainOutput, dedupChain)
	}

	commonArgs := []string{"-p", "IPv4", "-s", macAddr.String(), "-o", "veth+"}
	_, err = tool.EnsureRule(tool.Prepend, tool.TableFilter, dedupChain,
		append(commonArgs, "--ip-src", gateway.String(), "-j", "ACCEPT")...)
	if err != nil {
		trace.Wrap(err, "failed to ensure rule for packets from %q gateway to be accepted", ifaceName)

	}

	_, err = tool.EnsureRule(tool.Append, tool.TableFilter, dedupChain,
		append(commonArgs, "--ip-src", podCidr, "-j", "DROP")...)
	if err != nil {
		return trace.Wrap(err, "failed to ensure rule to drop packets from %v but with mac address of %q",
			podCidr, ifaceName)
	}

	return nil
}

func getIPv4Addr(iface net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, addr := range addrs {
		switch ipAddr := addr.(type) {
		case *net.IPNet:
			ip := ipAddr.IP.To4()
			if ip != nil {
				return ip, nil
			}
		}
	}

	return nil, trace.NotFound("no IPv4 addresses found for %q", iface.Name)
}

// ebtables chain to store deduplication rules
var dedupChain = tool.Chain("KUBE-DEDUP")

// promiscuousModeOn specifies the value of the promiscuous mode flag
// in the output of `ip link show dev <name>`
var promiscuousModeOn = []byte("PROMISC")

const cmdIP = "/sbin/ip"
