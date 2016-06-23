// +build !k8s

package utils

import (
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

func GetK8sLabels(conf NetConf, k8sargs K8sArgs) (map[string]string, error) {
	return nil, nil
}

func CmdAddK8s(args *skel.CmdArgs, conf NetConf) (*types.Result, error) {
	return nil, nil

}

func RunningUnderK8s(args *skel.CmdArgs) (bool, error) {
	return false, nil
}
