package ebtables

import (
	"os/exec"
	"regexp"
	"strings"

	"github.com/gravitational/trace"
)

const (
	cmd = "/sbin/ebtables"

	// Flag to show full mac in output. The default representation omits leading zeroes.
	fullMac = "--Lmac2"
)

// RulePosition describe the position of a rule relative to another: before or after
type RulePosition string

const (
	// Prepend defines a rule position to place a new rule before another
	Prepend RulePosition = "-I"
	// Append defines a rule position to place a new rule after another
	Append RulePosition = "-A"
)

// Table is an ebtables table
type Table string

const (
	// TableNAT identifies the nat table
	TableNAT Table = "nat"
	// TableNAT identifies the filter table
	TableFilter Table = "filter"
)

// Chain is an ebtables chain
type Chain string

const (
	// ChainPostrouting identifies the POSTROUTING chain
	ChainPostrouting Chain = "POSTROUTING"
	// ChainPrerouting identifies the PREROUTING chain
	ChainPrerouting Chain = "PREROUTING"
	// ChainOutput identifies the OUTPUT chain
	ChainOutput Chain = "OUTPUT"
	// ChainInput identifies the INPUT chain
	ChainInput Chain = "INPUT"
)

type operation string

const (
	opCreateChain operation = "-N"
	opFlushChain  operation = "-F"
	opDeleteChain operation = "-X"
	opListChain   operation = "-L"
	opAppendRule  operation = "-A"
	opPrependRule operation = "-I"
	opDeleteRule  operation = "-D"
)

// GetVersion returns the "X.Y.Z" semver string for ebtables
func GetVersion() (string, error) {
	// this doesn't access mutable state so we don't need to use the interface / runner
	bytes, err := exec.Command(cmd, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	versionMatcher := regexp.MustCompile("v([0-9]+\\.[0-9]+\\.[0-9]+)")
	match := versionMatcher.FindStringSubmatch(string(bytes))
	if len(match) == 0 {
		return "", trace.NotFound("no ebtables version found in string %s", bytes)
	}
	return match[1], nil
}

// EnsureRule checks if the specified rule is present and, if not, creates it.  If the rule existed, return true.
// WARNING: ebtables does not provide check operation like iptables do. Hence we have to do a string match of args.
// Input args must follow the format and sequence of ebtables list output. Otherwise, EnsureRule will always create
// new rules and causing duplicates.
func EnsureRule(position RulePosition, table Table, chain Chain, args ...string) (bool, error) {
	var exists bool
	fullArgs := makeFullArgs(table, opListChain, chain, fullMac)
	out, err := exec.Command(cmd, fullArgs...).CombinedOutput()
	if err == nil {
		exists = checkIfRuleExists(string(out), args...)
	}
	if !exists {
		fullArgs = makeFullArgs(table, operation(position), chain, args...)
		out, err := exec.Command(cmd, fullArgs...).CombinedOutput()
		if err != nil {
			return exists, trace.Wrap(err, "failed to ensure rule: %s", out)
		}
	}
	return exists, nil
}

// EnsureChain checks if the specified chain is present and, if not, creates it.  If the rule existed, return true.
func EnsureChain(table Table, chain Chain) (bool, error) {
	exists := true

	args := makeFullArgs(table, opListChain, chain)
	_, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		exists = false
	}

	if !exists {
		args = makeFullArgs(table, opCreateChain, chain)
		out, err := exec.Command(cmd, args...).CombinedOutput()
		if err != nil {
			return exists, trace.Wrap(err, "failed to ensure %q chain: %s", chain, out)
		}
	}
	return exists, nil
}

// FlushChain flushes the specified chain. Returns error if chain does not exist.
func FlushChain(table Table, chain Chain) error {
	fullArgs := makeFullArgs(table, opFlushChain, chain)
	out, err := exec.Command(cmd, fullArgs...).CombinedOutput()
	if err != nil {
		return trace.Wrap(err, "failed to flush %q chain %q: %s", string(table), string(chain), out)
	}
	return nil
}

// checkIfRuleExists takes the output of ebtables list chain and checks if the input rules exists
func checkIfRuleExists(listChainOutput string, args ...string) bool {
	rule := strings.Join(args, " ")
	for _, line := range strings.Split(listChainOutput, "\n") {
		if strings.TrimSpace(line) == rule {
			return true
		}
	}
	return false
}

func makeFullArgs(table Table, op operation, chain Chain, args ...string) []string {
	return append([]string{"-t", string(table), string(op), string(chain)}, args...)
}
