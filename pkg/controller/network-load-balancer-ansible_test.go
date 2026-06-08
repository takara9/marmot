package controller

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func TestNetworkLoadBalancerPlaybookTemplate_RendersBackendRules(t *testing.T) {
	tmpl, err := template.New("network-load-balancer-playbook").Parse(networkLoadBalancerPlaybookTemplate)
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}

	data := networkLoadBalancerPlaybookData{
		RemoteCIDR: "10.0.0.0/24",
		Listeners: []networkLoadBalancerPlaybookListener{
			{
				Name:        "tcp-80",
				ChainName:   "MARMOT-NLB-NLB1-TCP-80",
				Protocol:    "tcp",
				VipPort:     80,
				BackendPort: 8080,
				BackendRules: []networkLoadBalancerBackendRule{
					{IP: "172.16.8.11", Probability: "0.500000"},
					{IP: "172.16.8.12"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "iptables -t nat -A MARMOT-NLB-NLB1-TCP-80 -p tcp -m statistic --mode random --probability 0.500000 -j DNAT --to-destination 172.16.8.11:8080") {
		t.Fatalf("rendered playbook missing probabilistic DNAT rule: %s", out)
	}
	if !strings.Contains(out, "iptables -t nat -A MARMOT-NLB-NLB1-TCP-80 -p tcp -j DNAT --to-destination 172.16.8.12:8080") {
		t.Fatalf("rendered playbook missing final DNAT rule: %s", out)
	}
	if !strings.Contains(out, "iptables -t nat -I MARMOT-NLB-PREROUTING 1 -p tcp --dport 80 -j MARMOT-NLB-NLB1-TCP-80") {
		t.Fatalf("rendered playbook must bind listener chain via dedicated prerouting chain: %s", out)
	}
	if !strings.Contains(out, "iptables -t nat -A MARMOT-NLB-POSTROUTING -p tcp -d 172.16.8.11 --dport 8080 -j MASQUERADE") {
		t.Fatalf("rendered playbook missing postrouting masquerade rule: %s", out)
	}
	if !strings.Contains(out, "marmot-nlb-iptables-restore.service") {
		t.Fatalf("rendered playbook missing systemd restore unit for persisted iptables rules: %s", out)
	}
	if !strings.Contains(out, "iptables-save > /etc/marmot/iptables/nlb-rules.v4") {
		t.Fatalf("rendered playbook missing iptables-save persistence command: %s", out)
	}
	if !strings.Contains(out, "\n        iptables -t nat -A MARMOT-NLB-NLB1-TCP-80 -p tcp -m statistic --mode random --probability 0.500000 -j DNAT --to-destination 172.16.8.11:8080") {
		t.Fatalf("rendered playbook DNAT rule must stay indented inside shell block: %s", out)
	}
}

func TestNetworkLoadBalancerPlaybookTemplate_AllowsForwardWhenRemoteCIDREmpty(t *testing.T) {
	tmpl, err := template.New("network-load-balancer-playbook").Parse(networkLoadBalancerPlaybookTemplate)
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}

	data := networkLoadBalancerPlaybookData{
		Listeners: []networkLoadBalancerPlaybookListener{
			{
				Name:        "http",
				ChainName:   "MARMOT-NLB-NLB1-HTTP",
				Protocol:    "tcp",
				VipPort:     80,
				BackendPort: 80,
				BackendRules: []networkLoadBalancerBackendRule{
					{IP: "172.16.8.4"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "iptables -t filter -I MARMOT-NLB-FORWARD 1 -p tcp --dport 80 -j ACCEPT") {
		t.Fatalf("rendered playbook must allow forwarding when remoteCIDR is empty: %s", out)
	}
	if strings.Contains(out, "-s  -p tcp --dport 80 -j ACCEPT") {
		t.Fatalf("rendered playbook must not include empty source selector: %s", out)
	}
	if strings.Contains(out, "\n  done\n") {
		t.Fatalf("rendered playbook contains outdented shell terminator that breaks YAML: %s", out)
	}
}
