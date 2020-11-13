package portforward

import (
	"fmt"
	"strings"
)

type DaemonConfig struct {
	Ports map[string]*PortForwardConfig
}

type PortForwardConfig struct {
	Active          bool     `yaml:"active" json:"active"`
	LocalPort       int      `yaml:"localPort" json:"localPort,omitempty"`
	KubeConfig      string   `yaml:"kubeConfig" json:"kubeConfig,omitempty"`
	KubeContext     string   `yaml:"kubeContext" json:"kubeContext,omitempty"`
	TargetType      string   `yaml:"targetType" json:"targetType,omitempty"`
	TargetName      string   `yaml:"targetName" json:"targetName,omitempty"`
	TargetPort      int      `yaml:"targetPort" json:"targetPort,omitempty"`
	Namespace       string   `yaml:"namespace" json:"namespace,omitempty"`
	Args            []string `yaml:"args" json:"args,omitempty"`
}


func (p PortForwardConfig) String() string {
	return strings.Join(p.ToArgs(), " ")
}


func (p PortForwardConfig) ToArgs() []string {
	args := p.Args

	if len(args) == 0 {
		args = []string{"port-forward"}

		if p.Namespace != "" {
			args = append(args, "--namespace", p.Namespace)
		}

		if p.KubeContext != "" {
			args = append(args, "--context", p.KubeContext)
		}

		if p.KubeConfig != "" {
			args = append(args, "--kubeconfig", p.KubeConfig)
		}

		if p.TargetType != "" && p.TargetName != "" {
			args = append(args, p.TargetType+"/"+p.TargetName)
		} else {
			args = append(args, p.TargetName)
		}

		args = append(args, fmt.Sprintf("%d:%d", p.LocalPort, p.TargetPort))
	}

	return args
}
