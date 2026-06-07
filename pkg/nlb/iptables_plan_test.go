package nlb

import (
	"strings"
	"testing"
)

func TestBuildIPTablesRestoreScript_Basic(t *testing.T) {
	input := IPTablesPlanInput{
		BindPublicIP: "192.168.1.72",
		RemoteCIDR:   "192.168.1.0/24",
		Listeners: []Listener{
			{
				Name:               "tcp-service",
				Protocol:           "tcp",
				VipPort:            4080,
				SessionPersistence: true,
				Backends: []Backend{
					{Address: "172.16.8.2", Port: 3080},
					{Address: "172.16.8.3", Port: 3080},
				},
			},
		},
	}

	script, err := BuildIPTablesRestoreScript(input)
	if err != nil {
		t.Fatalf("BuildIPTablesRestoreScript() error = %v", err)
	}

	checks := []string{
		"*nat",
		":NLB_0 - [0:0]",
		"-A PREROUTING -d 192.168.1.72 -s 192.168.1.0/24 -p tcp --dport 4080 -m conntrack --ctstate NEW -j NLB_0",
		"sessionPersistence=true uses conntrack flow pinning",
		"--to-destination 172.16.8.2:3080",
		"--to-destination 172.16.8.3:3080",
		"COMMIT",
	}

	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Fatalf("script missing %q\n%s", check, script)
		}
	}
}

func TestBuildIPTablesRestoreScript_DefaultRemoteCIDR(t *testing.T) {
	input := IPTablesPlanInput{
		BindPublicIP: "192.168.1.72",
		Listeners: []Listener{
			{
				Name:     "udp-service",
				Protocol: "udp",
				VipPort:  4200,
				Backends: []Backend{{Address: "172.16.8.4", Port: 4100}},
			},
		},
	}

	script, err := BuildIPTablesRestoreScript(input)
	if err != nil {
		t.Fatalf("BuildIPTablesRestoreScript() error = %v", err)
	}
	if !strings.Contains(script, "-s 0.0.0.0/0") {
		t.Fatalf("script should include default source cidr: %s", script)
	}
}

func TestBuildIPTablesRestoreScript_CustomChainPrefix(t *testing.T) {
	input := IPTablesPlanInput{
		BindPublicIP: "192.168.1.72",
		ChainPrefix:  "NLB_ab123",
		Listeners: []Listener{
			{
				Name:     "tcp-service",
				Protocol: "tcp",
				VipPort:  4080,
				Backends: []Backend{{Address: "172.16.8.2", Port: 3080}},
			},
		},
	}

	script, err := BuildIPTablesRestoreScript(input)
	if err != nil {
		t.Fatalf("BuildIPTablesRestoreScript() error = %v", err)
	}
	if !strings.Contains(script, ":NLB_ab123_0 - [0:0]") {
		t.Fatalf("script should include custom chain name: %s", script)
	}
	if !strings.Contains(script, "-j NLB_ab123_0") {
		t.Fatalf("script should jump to custom chain name: %s", script)
	}
}

func TestBuildIPTablesRestoreScript_InvalidInput(t *testing.T) {
	_, err := BuildIPTablesRestoreScript(IPTablesPlanInput{})
	if err == nil {
		t.Fatalf("expected error for invalid input")
	}
}
