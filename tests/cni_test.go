package tests

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/dchest/uniuri"
	"os/exec"
	"fmt"
	"io"
	"bytes"
	"os"
)

func TestSingleNetworkHostLocal(t *testing.T) {
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

	container_id, ip := CreateContainer(netconf)
	fmt.Printf("container_id: %v, result: %v\n", container_id, ip)
	// What to assert on?

	// etcd things:
	// Profile is created with correct details
	// The endpoint is created in etcd

	// host things

	// netns things



	assert.True(t, true, "True is true!")

	DeleteContainer(netconf)
	// What to assert on?
	// profile still exists - nah
	// endpoint is removed
	// host things - route and interface are removed
	// netns is removed


}

// Test that both etcd_endpoints and etcd_authity can be used
// Test k8s
// test bad network name
// badly formatted netconf
// vary the MTU
// Existing endpoint



func CreateContainer(netconf string) (container_id, result string) {
	// Create a random "container ID"
	container_id = uniuri.New()
	netnspath := "/var/run/netns/" + container_id

	// Set up a network namespace suing the container ID and enable localhost.
	Cmd(fmt.Sprintf("ip netns add %s", container_id))
	Cmd(fmt.Sprintf("ip netns exec %s ip link set lo up ", container_id))

	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=ADD CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=eth0 CNI_PATH=../dist", container_id, netnspath)

	// Run the CNI plugin passing in the supplied netconf
	result = CmdWithStdin(fmt.Sprintf("%s ../dist/calico", cni_env), netconf)
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
		fmt.Println(err)
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

	if err = subProcess.Start(); err != nil {
		fmt.Println("An error occured: ", err)
	}
	io.WriteString(stdin, stdin_string)
	io.WriteString(stdin, "\n")

	stdin.Close()
	err = subProcess.Wait()
	if err != nil || stderr_buf.Len() != 0 {
		fmt.Println(err)
		fmt.Println(stdout_buf.String())
		fmt.Println("Processes completed STDERR:", stderr_buf.String())
		panic("some error found")
	}

	return stdout_buf.String()

}