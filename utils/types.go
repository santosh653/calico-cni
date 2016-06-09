package utils

import "github.com/containernetworking/cni/pkg/types"

type Policy struct {
	PolicyType              string `json:"type"`
	K8sApiRoot              string `json:"k8s_api_root"`
	K8sAuthToken            string `json:"k8s_auth_token"`
	K8sClientCertificate    string `json:"k8s_client_certificate"`
	K8sClientKey            string `json:"k8s_client_key"`
	K8sCertificateAuthority string `json:"k8s_certificate_authority"`
}

type NetConf struct {
	types.NetConf
	MTU           int  `json:"mtu"`
	EtcdAuthority string `json:"etcd_authority"`
	EtcdEndpoints string `json:"etcd_endpoints"`
	Policy        *Policy `json:"policy"`
}

type K8sArgs struct {
	K8S_POD_NAME               string
	K8S_POD_NAMESPACE          string
	K8S_POD_INFRA_CONTAINER_ID string
}
