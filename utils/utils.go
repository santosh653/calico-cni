package utils

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func LoadArgs(args string, container *K8sArgs) error {
	// TODO - I can get rid of this if this gets merged https://github.com/containernetworking/cni/pull/238
	if args == "" {
		return nil
	}

	pairs := strings.Split(args, ";")
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return fmt.Errorf("ARGS: invalid pair %q", pair)
		}
		keyString := kv[0]
		valueString := kv[1]
		switch {
		case keyString == "K8S_POD_INFRA_CONTAINER_ID":
			container.K8S_POD_INFRA_CONTAINER_ID = valueString
		case keyString == "K8S_POD_NAMESPACE":
			container.K8S_POD_NAMESPACE = valueString
		case keyString == "K8S_POD_NAME":
			container.K8S_POD_NAME = valueString
		}
	}
	return nil
}

func ValidateNetworkName(name string) error {
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_\.\-]+$`, name)
	if err != nil {
		return err
	}
	if !matched {
		return errors.New("Invalid characters detected in the given network name. " +
			"Only letters a-z, numbers 0-9, and symbols _.- are supported.")
	}
	return nil
}

func AddIgnoreUnknownArgs() {
	// Append the 'IgnoreUnknown=1' option to CNI_ARGS before calling the IPAM plugin. Otherwise, it will
	// complain about the Kubernetes arguments. See https://github.com/kubernetes/kubernetes/pull/24983
	cniArgs := "IgnoreUnknown=1"
	if os.Getenv("CNI_ARGS") != "" {
		cniArgs = fmt.Sprintf("%s;%s", cniArgs, os.Getenv("CNI_ARGS"))
	}
	os.Setenv("CNI_ARGS", cniArgs)
}
