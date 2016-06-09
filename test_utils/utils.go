package test_utils

import (
	"os/exec"
	"bytes"
	"io"
	"fmt"
	"os"
	"github.com/dchest/uniuri"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/ginkgo"
	"strings"
)

func RunIpam(netconf string) string {
	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=ADD CNI_CONTAINERID=whenever CNI_NETNS=whatever CNI_IFNAME=eth0 CNI_PATH=../dist")

	// Run the CNI plugin passing in the supplied netconf
	return CmdWithStdin(fmt.Sprintf("%s ../dist/%s-ipam", cni_env, os.Getenv("PLUGIN")), netconf)
}

func CreateContainer(netconf string) (container_id string, session *gexec.Session, err error) {
	// Create a random "container ID"
	container_id = uniuri.NewLen(10)
	netnspath := "/var/run/netns/" + container_id

	// Set up a network namespace suing the container ID and enable localhost.
	Cmd(fmt.Sprintf("ip netns add %s", container_id))
	Cmd(fmt.Sprintf("ip netns exec %s ip link set lo up ", container_id))

	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=ADD CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=eth0 CNI_PATH=dist", container_id, netnspath)

	// Run the CNI plugin passing in the supplied netconf
	subProcess := exec.Command("bash", "-c", fmt.Sprintf("%s dist/%s", cni_env, os.Getenv("PLUGIN")), netconf)
	stdin, err := subProcess.StdinPipe()
	if err != nil {
		panic("some error found")
	}

	io.WriteString(stdin, netconf)
	io.WriteString(stdin, "\n")
	stdin.Close()

	session, err = gexec.Start(subProcess, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	return
}

func DeleteContainer(netconf, container_id string) (session *gexec.Session, err error) {
	netnspath := "/var/run/netns/" + container_id
	// Set up the env for running the CNI plugin
	cni_env := fmt.Sprintf("CNI_COMMAND=DEL CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=eth0 CNI_PATH=dist", container_id, netnspath)

	// Run the CNI plugin passing in the supplied netconf
	subProcess := exec.Command("bash", "-c", fmt.Sprintf("%s dist/%s", cni_env, os.Getenv("PLUGIN")), netconf)
	stdin, err := subProcess.StdinPipe()
	if err != nil {
		panic("some error found")
	}

	io.WriteString(stdin, netconf)
	io.WriteString(stdin, "\n")
	stdin.Close()

	session, err = gexec.Start(subProcess, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	return
}

func Cmd(cmd string) string {
	ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("Running command [%s]\n", cmd)))
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		ginkgo.GinkgoWriter.Write(out)
		ginkgo.GinkgoWriter.Write(err.(*exec.ExitError).Stderr)
		ginkgo.Fail("Command failed")
	}
	return strings.TrimSpace(string(out))
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
	etcdCommand := fmt.Sprintf("etcdctl --endpoints http://%s:2379 get %s", os.Getenv("ETCD_IP"), path)
	return Cmd(etcdCommand)
	//session, err := gexec.Start(exec.Command("bash", "-c", etcdCommand), ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	//
	//if err != nil {
	//	fmt.Println(string(session.Out.Contents()))
	//	fmt.Println(string(err.(*exec.ExitError).Stderr))
	//	panic("some error found")
	//}
	//session.Wait()
	//
	//return session
}

func EtcdGetCommand(path string) string {
	//return "etcdctl", []string{"--endpoints", fmt.Sprintf("http://%s:2379", os.Getenv("ETCD_IP")), "get", path}
	return fmt.Sprintf("etcdctl --endpoints http://%s:2379 get %s", os.Getenv("ETCD_IP"), path)
}



