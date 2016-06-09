// +build !k8s

package utils

func GetK8sLabels(conf NetConf, k8sargs K8sArgs) (map[string]string, error) {
	return nil, nil
}