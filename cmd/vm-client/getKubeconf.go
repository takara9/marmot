package main

import (
	"fmt"
	"os"
	scp "github.com/povsister/scp"
	"io/ioutil"
	"log"
	"regexp"
	cf "github.com/takara9/marmot/pkg/config"
)


func GetKubeconf(cnf cf.MarmotConfig ) error {
	var master_node_ip string
	r, err := regexp.Compile("^master[0-9]")
	for _,v := range cnf.VMSpec {
		if r.MatchString(v.Name) {
			master_node_ip = v.PrivateIP
			break
		}
	}

	privPEM, err := ioutil.ReadFile(os.Getenv("HOME") + "/.ssh/id_rsa")
	if err != nil {
		log.Println("err = ",err)
		return err
	}
	sshConf, err := scp.NewSSHConfigFromPrivateKey("root", privPEM)
	if err != nil {
		log.Println("err = ",err)
		return err
	}

	hostport := fmt.Sprintf("%s:%d", master_node_ip, 22)
	scpClient, err := scp.NewClient(hostport, sshConf, &scp.ClientOption{})
	if err != nil {
		log.Println("err = ",err)
		return err
	}
	defer scpClient.Close()

	// リモートからファイル取得
	kubeconf := fmt.Sprintf("%s/admin.kubeconfig_%s", os.Getenv("PWD"), cnf.ClusterName)
	err = scpClient.CopyFileFromRemote("/etc/kubernetes/admin.conf", kubeconf, &scp.FileTransferOption{})
	if err != nil {
		log.Println("err = ",err)
		return err
	}
	return nil

}
