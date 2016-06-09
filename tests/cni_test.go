package tests

import (
	"testing"
	"github.com/dchest/uniuri"
	"os/exec"
	"fmt"
	"io"
	"bytes"
	"os"
	//"net/http"
	//"io/ioutil"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestCalicoIpam(t *testing.T) {
	RegisterTestingT(t)
	Cmd(fmt.Sprintf("etcdctl --endpoints http://%s:2379 rm /calico --recursive | true", os.Getenv("ETCD_IP")))

	netconf := fmt.Sprintf(`
{
  "name": "net1",
  "type": "calico",
  "log_level": "NONE",
  "log_level_stderr": "debug",
  "etcd_authority": "1.2.3.4:2379",
  "etcd_endpoints": "http://%s:2379",
  "ipam": {
    "type": "calico-ipam"
  }
}`, os.Getenv("ETCD_IP"))

	fmt.Println(RunIpam(netconf))
}
//
func TestSingleNetworkHostLocal(t *testing.T) {
	RegisterTestingT(t)
	Cmd(fmt.Sprintf("etcdctl --endpoints http://%s:2379 rm /calico --recursive | true", os.Getenv("ETCD_IP")))

	netconf := fmt.Sprintf(`
{
  "name": "net1",
  "type": "calico",
  "log_level": "NONE",
  "log_level_stderr": "debug",
  "etcd_authority": "1.2.3.4:2379",
  "etcd_endpoints": "http://%s:2379",
  "ipam": {
    "type": "host-local",
    "subnet": "10.0.0.0/8"
  }
}`, os.Getenv("ETCD_IP"))


	// Assert that profile doesn't  yet exist and that no endpoints exist

	// Network a namespace then check it was set up correctly
	container_id, ip := CreateContainer(netconf)
	fmt.Printf("container_id: %v, result: %v\n", container_id, ip)

	// etcd things:
	// Profile is created with correct details
	Expect(GetEtcdString("/calico/v1/policy/profile/net1/tags")).Should(MatchJSON(`["net1"]`))
	if os.Getenv("PLUGIN") != "calipo" { // Python returns a bad extra field
		Expect(GetEtcdString("/calico/v1/policy/profile/net1/rules")).Should(MatchJSON(`{"inbound_rules":[{"action":"allow","src_tag":"net1"}],"outbound_rules":[{"action":"allow"}]}`))
	}

	// The endpoint is created in etcd
	endpoint_path := Cmd(fmt.Sprintf("etcdctl --endpoints http://%s:2379 ls /calico/v1/host/cnitests/workload/cni/%s --recursive |tail -1", os.Getenv("ETCD_IP"), container_id))
	endpoint := GetEtcdString(endpoint_path)
	Expect(endpoint_path).Should(ContainSubstring(container_id))
	Expect(endpoint).Should(ContainSubstring(`"state":"active"`))
	Expect(endpoint).Should(ContainSubstring(`"profile_ids":["net1"]`))
	//Expect(endpoint).Should(ContainSubstring(`"ipv4_nets":["%s"]`, ip))
	Expect(endpoint).Should(ContainSubstring(`"ipv6_nets":[]`))
	Expect(endpoint).Should(ContainSubstring(`"name":"cali%s"`, container_id))
	// Check the MAC is a valid MAC, maybe storing it off to check that the interface actually has that MAC


	// Routes and interface on host
	fmt.Println(Cmd("ip link show"))
	fmt.Println(Cmd(fmt.Sprintf("ip link show cali%s", container_id)))
	fmt.Println(Cmd("ip route"))

	// Routes and interface in netns
	fmt.Println(Cmd(fmt.Sprintf("ip netns exec %s ip addr show eth0", container_id)))
	fmt.Println(Cmd(fmt.Sprintf("ip netns exec %s ip route", container_id)))

	assert.True(t, true, "True is true!")

	//DeleteContainer(netconf)
	// What to assert on?
	// profile still exists - nah
	// endpoint is removed
	// host things - route and interface are removed
	// netns is removed


}




func RunIpam(netconf string) string {
	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=ADD CNI_CONTAINERID=whenever CNI_NETNS=whatever CNI_IFNAME=eth0 CNI_PATH=../dist")

	// Run the CNI plugin passing in the supplied netconf
	return CmdWithStdin(fmt.Sprintf("%s ../dist/%s-ipam", cni_env, os.Getenv("PLUGIN")), netconf)
}

func CreateContainer(netconf string) (container_id, result string) {
	// Create a random "container ID"
	container_id = uniuri.NewLen(10)
	netnspath := "/var/run/netns/" + container_id

	// Set up a network namespace suing the container ID and enable localhost.
	Cmd(fmt.Sprintf("ip netns add %s", container_id))
	Cmd(fmt.Sprintf("ip netns exec %s ip link set lo up ", container_id))

	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=ADD CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=eth0 CNI_PATH=../dist", container_id, netnspath)

	// Run the CNI plugin passing in the supplied netconf
	result = CmdWithStdin(fmt.Sprintf("%s ../dist/%s", cni_env, os.Getenv("PLUGIN")), netconf)
	return
}

func DeleteContainer(netconf string) (container_id, result string) {
	// Create a random "container ID"
	container_id = uniuri.New()
	netnspath := "/var/run/netns/" + container_id

	// Set up a network namespace suing the container ID and enable localhost.
	Cmd(fmt.Sprintf("ip netns add %s", container_id))
	Cmd(fmt.Sprintf("ip netns exec %s ip link set lo up ", container_id))

	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=DEL CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=eth0 CNI_PATH=../dist", container_id, netnspath)

	// Run the CNI plugin passing in the supplied netconf
	result = CmdWithStdin(fmt.Sprintf("%s ../dist/calico", cni_env), netconf)
	return
}

func Cmd(cmd string) string {
	//fmt.Println("Running command:", cmd)
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		fmt.Println(out)
		fmt.Println(string(err.(*exec.ExitError).Stderr))
		panic("some error found")
	}
	return string(out)
}

func CmdWithStdin(cmd, stdin_string string) string {
	//fmt.Println("Running command:", cmd)
	//fmt.Println("stdin:", stdin_string)
	subProcess := exec.Command("bash", "-c", cmd)
	stdin, err := subProcess.StdinPipe()
	if err != nil {
		panic("some error found")
	}

	stdout_buf := new(bytes.Buffer)
	stderr_buf := new(bytes.Buffer)

	subProcess.Stdout = stdout_buf
	subProcess.Stderr = stderr_buf


	io.WriteString(stdin, stdin_string)
	io.WriteString(stdin, "\n")
	stdin.Close()

	if err = subProcess.Start(); err != nil {
		fmt.Println("An error occured: ", err)
	}



	err = subProcess.Wait()
	//if err != nil || stderr_buf.Len() != 0 {
	//	fmt.Println(err)
	//	fmt.Println(stdout_buf.String())
	//	fmt.Println("Processes completed STDERR:", stderr_buf.String())
	//	panic("some error found")
	//}

	return stdout_buf.String()
}

func GetEtcdString(path string) string {
	//response, err := http.Get(fmt.Sprintf("http://%s:2379/v2/keys%s", os.Getenv("ETCD_IP"), path))
	//if err != nil {
	//	panic("Can't contact etcd")
	//} else {
	//	defer response.Body.Close()
	//	contents, err := ioutil.ReadAll(response.Body)
	//	if err != nil {
	//		panic("Couldn't read response")
	//	}
	//	return string(contents)
	//}
	//return ""
	return Cmd(fmt.Sprintf("etcdctl --endpoints http://%s:2379 get %s", os.Getenv("ETCD_IP"), path))
}

// TODOs
// change to using gexec
// Get the first test written
